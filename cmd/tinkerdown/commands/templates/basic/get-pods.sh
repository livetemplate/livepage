#!/usr/bin/env bash
# Fetch Kubernetes pods and format as JSON array
# Customize this command for your cluster

kubectl get pods -o json 2>/dev/null | jq '[.items[] | {
  name: .metadata.name,
  namespace: .metadata.namespace,
  status: .status.phase,
  ready: (.status.containerStatuses[0].ready // false)
}]' 2>/dev/null || echo '[]'
