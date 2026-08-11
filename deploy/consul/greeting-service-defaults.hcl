# Declares greeting's protocol as HTTP so the ingress gateway's HTTP
# listener (see ingress-gateway.hcl) can route to it; Connect proxies
# default to plain TCP otherwise.
Kind     = "service-defaults"
Name     = "greeting"
Protocol = "http"
