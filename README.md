# OpenFGA Local k3s Deployment Guide

This guide provides the exact commands and files to deploy OpenFGA (with PostgreSQL) and the `document-service` on your local k3s cluster, using Traefik as the authorization gateway.

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

**3. Connect to the API:**
If the playground asks for an API URL, use: `http://localhost:8080`.

---

---

## Step 3: Create the OpenFGA Authorization Model

This model mirrors your app's routes: users can `read`, `write`, or `own` documents.

**File:** `platform-infra/openfga/model.fga`
```fga
model
  schema 1.1

type user

type document
  relations
    define reader: [user]
    define writer: [user]
    define owner: [user]
```

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

echo ""
echo "Done! Set these in your document-service deployment:"
echo "  OPENFGA_STORE_ID=$STORE_ID"
echo "  OPENFGA_API_URL=$OPENFGA_URL"
```

**Run it (after port-forwarding):**
```bash
kubectl port-forward svc/openfga 8080:8080 -n openfga &
chmod +x /home/corganfuzz/fga/platform-infra/openfga/setup-store.sh
/home/corganfuzz/fga/platform-infra/openfga/setup-store.sh
```
> Note: This script requires the `fga` CLI for writing the model. Install it with: `go install github.com/openfga/cli/cmd/fga@latest`

---

## Step 4: Helm Chart for the Document Service

**File:** `platform-infra/helm/document-service/Chart.yaml`
```yaml
apiVersion: v2
name: document-service
description: Helm chart for the document-service Go app
type: application
version: 0.1.0
appVersion: "1.0.0"
```

**File:** `platform-infra/helm/document-service/values.yaml`
```yaml
image:
  repository: document-service   # Update to your registry path, e.g. localhost:5000/document-service
  tag: latest
  pullPolicy: IfNotPresent

service:
  port: 8090

ingress:
  host: document-service.local   # Add this to your /etc/hosts pointing to your k3s node IP

openfga:
  apiUrl: "http://openfga.openfga.svc.cluster.local:8080"
  storeId: "REPLACE_WITH_YOUR_STORE_ID"   # Get this from setup-store.sh output
```

**File:** `platform-infra/helm/document-service/templates/deployment.yaml`
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Release.Name }}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: {{ .Release.Name }}
  template:
    metadata:
      labels:
        app: {{ .Release.Name }}
    spec:
      containers:
        - name: document-service
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          ports:
            - containerPort: 8090
          env:
            - name: OPENFGA_API_URL
              value: {{ .Values.openfga.apiUrl }}
            - name: OPENFGA_STORE_ID
              value: {{ .Values.openfga.storeId }}
          livenessProbe:
            httpGet:
              path: /healthz
              port: 8090
```

**File:** `platform-infra/helm/document-service/templates/service.yaml`
```yaml
apiVersion: v1
kind: Service
metadata:
  name: {{ .Release.Name }}
spec:
  selector:
    app: {{ .Release.Name }}
  ports:
    - port: 8090
      targetPort: 8090
```

**File:** `platform-infra/helm/document-service/templates/ingress.yaml`
```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: {{ .Release.Name }}
  annotations:
    # Reference the Traefik ForwardAuth middleware defined below
    traefik.ingress.kubernetes.io/router.middlewares: "{{ .Release.Namespace }}-openfga-auth@kubernetescrd"
spec:
  rules:
    - host: {{ .Values.ingress.host }}
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: {{ .Release.Name }}
                port:
                  number: 8090
```

**File:** `platform-infra/helm/document-service/templates/traefik-middleware.yaml`
```yaml
apiVersion: traefik.containo.us/v1alpha1
kind: Middleware
metadata:
  name: openfga-auth
spec:
  forwardAuth:
    # This is the OpenFGA service internal URL (port 8080 is the HTTP API)
    # In a real setup, this would point to an auth-adapter that translates
    # Traefik's headers into an OpenFGA /check POST request.
    address: "http://openfga.openfga.svc.cluster.local:8080/"
    trustForwardHeader: true
    authResponseHeaders:
      - "X-Auth-User"
```

> [!WARNING]
> Traefik's `ForwardAuth` sends a `GET` request to the `address`. OpenFGA's `/check` requires a structured `POST` body. In practice, you need a lightweight **auth-adapter** service between Traefik and OpenFGA that converts the incoming request context into the FGA tuple check. This is the next step once the base deployment is working.

**Deploy the document-service:**
```bash
# First, build and load the image into k3d (since there's no registry)
# Build from the project root to include go.mod
docker build -t document-service:latest -f /home/corganfuzz/fga/document-service/app/Dockerfile /home/corganfuzz/fga/document-service

# Import into k3d (replace 'localHTC' with your cluster name if different)
k3d image import document-service:latest -c localHTC

# Then deploy with Helm
helm upgrade --install document-service \
  /home/corganfuzz/fga/platform-infra/helm/document-service \
  --namespace document-service \
  --create-namespace \
  --set openfga.storeId=<STORE_ID_FROM_SETUP_SCRIPT>
```

---

## Step 5: Smoke Test

Because `document-service.local` is a custom domain, you need to tell your machine how to resolve it.

**Option A: Update /etc/hosts (Recommended)**
Add this line to your `/etc/hosts` file:
```text
127.0.0.1 document-service.local
```
Then run:
```bash
curl http://document-service.local/healthz
```

**Option B: Use curl --resolve**
If you don't want to edit `/etc/hosts`, use this command:
```bash
curl --resolve document-service.local:80:127.0.0.1 http://document-service.local/healthz
```

**Expected Output:** `{"status":"ok"}`
