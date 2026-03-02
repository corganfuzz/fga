# OpenFGA Local k3d Deployment Guide

This guide provides the exact commands and files to deploy OpenFGA (with PostgreSQL) and the `document-service` on your local k3d cluster, using Traefik as the authorization gateway.

## Architecture

```mermaid
sequenceDiagram
    participant User
    participant Traefik as Traefik (Ingress)
    participant FGA as OpenFGA (AuthN/AuthZ)
    participant DocSvc as Document Service
    participant DB as PostgreSQL (FGA Store)

    User->>Traefik: Request (GET/PUT/DELETE)
    Traefik->>FGA: GET /check (ForwardAuth)
    FGA-->>Traefik: 200 OK / 403 Forbidden
    
    alt Authorized
        Traefik->>DocSvc: Forward Request
        DocSvc-->>User: JSON Response
    else Forbidden
        Traefik-->>User: 403 Forbidden
    end

    User->>Traefik: Share Document (POST /share)
    Traefik->>DocSvc: Forward Request
    DocSvc->>FGA: SDK Write (Create Tuple)
    FGA->>DB: Persist Tuple
    DocSvc-->>User: 200 OK
```

## Kubernetes Infrastructure

This diagram shows how the resources are organized across Namespaces and how the Traefik Middleware connects them.

```mermaid
graph TD
    User([User / Developer])
    
    subgraph Cluster ["k3d Cluster"]
        subgraph NS_Doc ["Namespace: document-service"]
            Ingress["Ingress (document-service.local)"]
            DocSvc["Service: document-service"]
            MW["Middleware: openfga-auth"]
            DocPod["Deployment: document-service"]
        end

        subgraph NS_FGA ["Namespace: openfga"]
            FGASvc["Service: openfga"]
            FGAPod["Deployment: openfga"]
            PGDB[(PostgreSQL)]
        end
    end

    Output_JSON([JSON Document/Status])
    Output_FGA([Relationship Tuple])

    User -- "GET/PUT/DELETE" --> Ingress
    User -- "POST /share" --> Ingress
    
    Ingress -- 1. Auth Check --> MW
    MW -- 2. /check --> FGASvc
    
    Ingress -- 3. Forward --> DocSvc
    DocSvc --> DocPod
    
    DocPod -- "Update Memory" --> Output_JSON
    DocPod -- "Create Tuple (SDK)" --> FGASvc
    FGASvc --> FGAPod
    FGAPod --> PGDB
    PGDB --> Output_FGA
```

## Final Folder Structure

```text
platform-infra/
├── helm/
│   ├── openfga/
│   │   └── values.yaml            # OpenFGA config pointing to Postgres
│   └── document-service/
│       ├── Chart.yaml             # Helm chart for your Go app
│       ├── values.yaml            # App config (image, port, ingress)
│       └── templates/
│           ├── deployment.yaml
│           ├── service.yaml
│           ├── ingress.yaml
│           └── traefik-middleware.yaml
└── openfga/
    ├── model.fga                  # Authorization rules (DSL)
    └── setup-store.sh             # Create store + write model
```

---

## Step 1: Create the Directory Structure

```bash
mkdir -p /home/corganfuzz/fga/platform-infra/helm/openfga
mkdir -p /home/corganfuzz/fga/platform-infra/helm/document-service/templates
mkdir -p /home/corganfuzz/fga/platform-infra/openfga
```

---

## Step 2: Deploy OpenFGA + PostgreSQL

Use the standard Helm repository to avoid OCI errors.

**Add the Repository:**
```bash
helm repo add openfga https://openfga.github.io/helm-charts
helm repo update
```

**File:** `platform-infra/helm/openfga/values.yaml`
```yaml
datastore:
  engine: postgres
  uri: "postgres://postgres:password@openfga-postgresql.openfga.svc.cluster.local:5432/postgres?sslmode=disable"

postgresql:
  enabled: true
  image:
    tag: latest
  auth:
    postgresPassword: "password"
    database: "postgres"
```

**Deploy command:**
```bash
helm upgrade --install openfga openfga/openfga \
  --namespace openfga \
  --create-namespace \
  -f /home/corganfuzz/fga/platform-infra/helm/openfga/values.yaml
```

**Verify:**
```bash
kubectl get pods -n openfga
# Wait until openfga and openfga-postgresql pods are Running
```

---

## Accessing the OpenFGA Playground

The OpenFGA Playground is a web-based UI to visualize your models and test tuples. It is included by default.

**1. Port-Forward the Playground (3000) AND the API (8080):**
The Playground runs in your browser but needs to "talk" to the OpenFGA API. You must forward both:

```bash
# In one terminal:
kubectl port-forward svc/openfga 3000:3000 -n openfga

# In another terminal:
kubectl port-forward svc/openfga 8080:8080 -n openfga
```

**2. Open in Browser:**
Go to [http://localhost:3000](http://localhost:3000).

---

## Step 3: Create the OpenFGA Authorization Model

This model mirrors your app's routes: users can `read`, `write`, or `own` documents.

### Understanding the Model (`model.fga`)

OpenFGA uses a **Relationship-Based Access Control (ReBAC)** model. Here is what each part of this model does:

```fga
model
  schema 1.1

type user

type document
  relations
    define owner: [user]
    define reader: [user]
    define writer: [user]
```

*   **`type user`**: Defines a fundamental entity (the "subject"). In this case, an individual user.
*   **`type document`**: Defines the resource being protected.
*   **`relations`**: These are "slots" where you can place users to give them permissions.
    *   **`owner`**: Only a user assigned as an `owner` via a tuple will have this relation.
    *   **`reader`**: Only a user assigned as a `reader` via a tuple will have this relation.
    *   **`writer`**: Only a user assigned as a `writer` via a tuple will have this relation.

> [!NOTE]
> Currently, these relations are **independent**. In a production model, you would typically define inheritance, e.g., `define reader: [user] or writer`. This would mean any `writer` is automatically a `reader`.

**File:** `platform-infra/openfga/setup-store.sh`
```bash
#!/bin/bash
set -e

OPENFGA_URL=${OPENFGA_URL:-"http://localhost:8080"}
STORE_NAME="document-service"
MODEL_FILE="$(dirname "$0")/model.fga"

echo "Creating store..."
STORE_RESPONSE=$(curl -s -X POST "$OPENFGA_URL/stores" \
  -H "Content-Type: application/json" \
  -d "{\"name\": \"$STORE_NAME\"}")

STORE_ID=$(echo "$STORE_RESPONSE" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
echo "Store ID: $STORE_ID"

echo "Writing model..."
fga model write --store-id "$STORE_ID" --file "$MODEL_FILE" --api-url "$OPENFGA_URL"
```

---

## Step 4: Real Document Service Implementation

Your Go application uses the **OpenFGA Go SDK** to manage relationships and maintains an **in-memory store** for document content.

**1. Building and Deploying:**
```bash
# Building from the root (to include go.mod)
docker build -t document-service:latest -f /home/corganfuzz/fga/document-service/app/Dockerfile /home/corganfuzz/fga/document-service

# Import into k3d
k3d image import document-service:latest -c localHTC

# Deploy with Helm
helm upgrade --install document-service \
  /home/corganfuzz/fga/platform-infra/helm/document-service \
  --namespace document-service \
  --create-namespace \
  --set openfga.storeId=<STORE_ID_FROM_SETUP_SCRIPT>
```

---

## Step 5: Smoke Test (Ingress)

```bash
curl --resolve document-service.local:80:127.0.0.1 http://document-service.local/healthz
# Expected Output: {"status":"ok"}
```

---

## Step 6: Verify Real Implementation (Functional Test)

Verify document storage and OpenFGA sharing:

**1. Create/Update a Document:**
```bash
# Port-forward first: kubectl port-forward svc/document-service 8090:8090 -n document-service
curl -X PUT http://localhost:8090/documents/doc1 -d '{"content":"Hello FGA"}'
# Output: {"document":"doc1","status":"updated"}
```

**2. Retrieve the Document:**
```bash
curl http://localhost:8090/documents/doc1
# Output: {"id":"doc1","content":"Hello FGA"}
```

**3. Share the Document (OpenFGA Tuple Write):**
```bash
curl -X POST http://localhost:8090/documents/doc1/share \
  -H "Content-Type: application/json" \
  -d '{"user":"fga_user", "relation":"writer"}'
# Output: {"as":"writer","document":"doc1","shared_with":"fga_user","status":"ok"}
```
*(Verify the tuple in your OpenFGA Playground!)*
