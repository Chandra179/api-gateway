# Ingress gateway config entry: the Envoy ingress gateway's :8080 listener
# routes HTTP traffic to the "greeting" service over the Connect mesh. This
# is the client-facing edge that replaced Traefik.
Kind = "ingress-gateway"
Name = "ingress-gateway"

Listeners = [
  {
    Port     = 8080
    Protocol = "http"
    Services = [
      { Name = "greeting" }
    ]
  }
]
