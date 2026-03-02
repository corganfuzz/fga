# OpenFGA Local k3d Deployment Guide

This guide provides the exact commands and files to deploy OpenFGA (with PostgreSQL) and the `document-service` on your local k3d cluster, using Traefik as the authorization gateway.

## Architecture (Logical Flow)

```mermaid
sequenceDiagram
    autonumber
    participant User
    participant Traefik as Traefik (Ingress)
    participant Auth0 as Auth0 (Authentication)
    participant FGA as OpenFGA (Authorization)
    participant DocSvc as Document Service
    participant DB as PostgreSQL

    User->>Traefik: 1. Request (GET /doc1)
    
    rect rgb(200, 230, 255)
    Note over Traefik,Auth0: Authentication Layer
    Traefik->>Auth0: 2. Redirect to Login (if no session)
    User->>Auth0: 3. Authenticate
    Auth0-->>Traefik: 4. JWT Callback
    end

    rect rgb(230, 255, 230)
    Note over Traefik,FGA: Authorization Layer
    Traefik->>FGA: 5. check(user, read, doc1)
    FGA-->>Traefik: 6. Allowed / Denied
    end
    
    alt Allowed
        Traefik->>DocSvc: 7. Forward Request
        DocSvc-->>User: 200 OK (Document Content)
    else Denied
        Traefik-->>User: 403 Forbidden
    end
```

## Kubernetes Infrastructure (Vertical Map)

```mermaid
graph TD
    User([User / Developer])
    
    subgraph Cluster ["k3d Cluster"]
        direction TB
        
        subgraph NS_Doc ["Namespace: document-service"]
            Ingress["Ingress (document-service.local)"]
            DocSvc["Service: document-service"]
            
            subgraph Security_Chain ["Chained Security"]
                MW_Err["MW: auth0-errors"]
                MW_Auth["MW: auth0-proxy"]
                MW_FGA["MW: openfga-auth"]
            end
            
            DocPod["Pod: document-service"]
            OAuth2["Pod: oauth2-proxy"]
        end

        subgraph NS_FGA ["Namespace: openfga"]
            FGASvc["Service: openfga"]
            FGAPod["Pod: openfga"]
            PGDB[(PostgreSQL)]
        end
    end

    %% Flow
    User -- "HTTP Request" --> Ingress
    Ingress --> Security_Chain
    
    Security_Chain --> MW_Err
    MW_Err -. "401 Redirect" .-> OAuth2
    
    Security_Chain --> MW_Auth
    MW_Auth -- "Validate Session" --> OAuth2
    
    Security_Chain --> MW_FGA
    MW_FGA -- "Check Tuple" --> FGASvc
    
    Security_Chain --> DocSvc
    DocSvc --> DocPod
    
    FGASvc --> FGAPod
    FGAPod --> PGDB
```

---

## Step 0: Deploy Auth0 Authentication (oauth2-proxy)

To keep your cluster secure, Auth0 credentials are split into a public `values.yaml` and a private `secrets.yaml` (which is gitignored).

**1. Create your secret file:**
`xinfra/helm/oauth2-proxy/secrets.yaml`
```yaml
config:
  clientID: "YOUR_CLIENT_ID"
  clientSecret: "YOUR_CLIENT_SECRET"
  cookieSecret: "YOUR_RANDOM_COOKIE_SECRET"
```

**2. Deploy the proxy:**
```bash
helm repo add oauth2-proxy https://oauth2-proxy.github.io/charts
helm repo update

helm upgrade --install oauth2-proxy oauth2-proxy/oauth2-proxy \
  --namespace document-service \
  -f /home/corganfuzz/fga/xinfra/helm/oauth2-proxy/values.yaml \
  -f /home/corganfuzz/fga/xinfra/helm/oauth2-proxy/secrets.yaml
```

---

## Final Folder Structure

```text
xinfra/
└── helm/
    ├── monitoring/
    │   └── values.yaml            # Prometheus/Grafana config
    ├── oauth2-proxy/
    │   ├── values.yaml            # Public OIDC config
    │   └── secrets.yaml           # PRIVATE (Gitignored) Auth0 keys
    └── document-service/
        └── templates/
            ├── oauth2-ingress.yaml # /oauth2 path routing
            ├── error-middleware.yaml # 401 -> login redirect
            └── auth0-middleware.yaml # Traefik -> Proxy handshake
```

---

## 📊 Monitoring (Observability)

The cluster is equipped with a full Prometheus & Grafana stack for real-time monitoring.

### 1. View Cluster Metrics (Grafana)

**Port-forward the Grafana service:**
```bash
# Use local port 4000 as requested
kubectl port-forward svc/prometheus-grafana 4000:80 -n monitoring
```

**Access the UI:**
- **URL**: [http://localhost:4000](http://localhost:4000)
- **User**: `admin`
- **Password**: `admin`

### 2. View Raw Metrics (Prometheus UI)
If you want to query the raw metrics directly:
```bash
kubectl port-forward svc/prometheus-kube-prometheus-prometheus 9090:9090 -n monitoring
```
- **URL**: [http://localhost:9090](http://localhost:9090)

### 3. Import Official OpenFGA Dashboard
The community ID `19471` can sometimes be out of sync. Use the official JSON provided by the OpenFGA team:
1.  In Grafana, go to **Dashboards -> Import**.
2.  **Upload JSON file**: Use the file at [xinfra/helm/monitoring/openfga-dashboard.json](file:///home/corganfuzz/fga/xinfra/helm/monitoring/openfga-dashboard.json).
3.  Select `Prometheus` as the datasource and click **Import**.

### 4. Generate Traffic (See Data in Dashboard)
If your dashboard is empty, you need to generate some checks. Run the included script:
```bash
# In a new terminal (port-forward OpenFGA first):
kubectl port-forward svc/openfga 8080:8080 -n openfga

# Run the traffic generator:
./xinfra/generate-fga-traffic.sh
```

**What is this doing?**
The script is a "Check Simulator". It does the following:
1.  **Iterates rapidly**: Sends a `/check` Request to OpenFGA every 0.1 seconds.
2.  **Randomizes Subjects/Objects**: It picks random users (e.g., `user:42`) and random docs (e.g., `document:doc12`) and asks OpenFGA: *"Does this user have this relation (reader/writer) on this doc?"*
3.  **Generates Telemetry**: These requests force OpenFGA to perform internal lookups, which triggers:
    - **Request Rate Counter**: Shows how many checks/sec are happening.
    - **Latency Histograms**: Shows how long the DB/Cache takes to answer.
    - **Store Activity**: Updates metrics specific to your `STORE_ID`.

**What are you seeing in Grafana?**
- **Check Request Rate**: You'll see a steady line or "hump" at ~10 requests per second.
- **Check Latency (P99)**: You'll see the 99th percentile of response times (usually <10ms locally).
- **HTTP/gRPC Status Codes**: A sea of 200 OKs.

### 4.5. Simulate Errors (See 4xx in Dashboard)
To verify that your alerts and and error panels work, run the error simulator:
```bash
# Run the error generator:
./xinfra/generate-fga-errors.sh
```

**What is this doing?**
- **404 Not Found**: Sends requests with a bogus `STORE_ID`.
- **400 Bad Request**: Sends malformed tuples (missing required fields).
- **Undefined Relations**: Asks for permissions that don't exist in your model.

**What will you see in Grafana?**
- **Error Rate (Non-2xx)**: You'll see a sharp spike in the error count.
- **HTTP status codes**: A mix of `400` and and `404` appearing in the "Status Codes" pie chart or time-series.

---
### 5. Monitoring your Services
The `document-service` is now instrumented with **Prometheus client_golang**. 
- **Scraper**: Automated via `ServiceMonitor` (check `xinfra/helm/document-service/templates/servicemonitor.yaml`).
- **Endpoint**: `/metrics` on port `8090`.

---

## Auth0 Implementation Details

The authentication layer follows a **ForwardAuth** pattern:

1.  **Traefik Ingress**: Catches all requests to `document-service.local`.
2.  **`auth0-errors` Middleware**: If the proxy returns a `401 Unauthorized`, this middleware catches it and redirects your browser to `/oauth2/start`.
3.  **`auth0-proxy` Middleware**: This is the "ForwardAuth" check. It asks `oauth2-proxy`: *"Is this user logged in?"*
4.  **`oauth2-proxy`**: 
    - If **Yes**: It returns 200 OK and passes the user session/JWT back to Traefik.
    - If **No**: It returns 401, triggering the error redirect above.

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

The OpenFGA Playground is a web-based UI to visualize your models and test tuples.

**1. Port-Forward the Playground (3000) AND the API (8080):**
```bash
# Terminal 1:
kubectl port-forward svc/openfga 3000:3000 -n openfga

# Terminal 2:
kubectl port-forward svc/openfga 8080:8080 -n openfga
```

**2. Open in Browser:**
Go to [http://localhost:3000](http://localhost:3000).

---

## Step 3: Create the OpenFGA Authorization Model

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

**1. Building and Deploying:**
```bash
# Building
docker build -t document-service:latest -f /home/corganfuzz/fga/document-service/app/Dockerfile /home/corganfuzz/fga/document-service

# Import into k3d
k3d image import document-service:latest -c localHTC

# Deploy
helm upgrade --install document-service \
  /home/corganfuzz/fga/platform-infra/helm/document-service \
  --namespace document-service \
  --create-namespace \
  --set openfga.storeId=<YOUR_STORE_ID>
```

---

## Step 6: Testing the Implementation

### A. Testing through the Secure Gateway (Ingress)

This test validates the **Auth0 + OpenFGA** security chain.

1.  **Browser Test**: Open `http://document-service.local/`.
    *   **Success**: You are redirected to Auth0, you log in, and then you see the document status.
2.  **CLI Test**:
    ```bash
    curl -I --resolve document-service.local:80:127.0.0.1 http://document-service.local/
    ```
    *   **Success**: You receive a `302 Found` (Redirect to Auth0).

### B. Testing Direct Service (Admin/Debug Bypass)

If you want to test the document service logic directly (e.g., creating documents without needing a browser), you can **bypass** the Ingress by using a `port-forward`.

**1. Port-forward the service:**
```bash
kubectl port-forward svc/document-service 8090:8090 -n document-service
```

**2. Run direct CRUD commands:**
These commands go straight to your Go app, bypassing Traefik's Auth0/OpenFGA checks.

```bash
# Create/Update a Document
curl -X PUT http://localhost:8090/documents/doc1 -d '{"content":"Hello Secure FGA"}'

# Retrieve the Document
curl http://localhost:8090/documents/doc1

# Share the Document (Writes to OpenFGA)
curl -X POST http://localhost:8090/documents/doc1/share \
  -H "Content-Type: application/json" \
  -d '{"user":"fga_user", "relation":"writer"}'
```
