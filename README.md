# External Processing examples

Example gRPC services that implement the interface used by Envoy's [External Processing filter](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/ext_proc_filter)

Currently the examples in this repo are used to support the Gloo Gateway [ExtProc documentation](https://docs.solo.io/gateway/latest/traffic-management/extproc/header-manipulation/) and Solo [blogs](https://www.solo.io/blog/extproc-info-leakage/).

These examples should only be considered sample code for Gloo Gateway enterprise users. They are not a supported component of any Gloo Gateway product. Do NOT use any of these examples in production systems.

# Change from original

As a proof of concept:
- check validity of request and response `api-version` header or set a default value,
- add additional headers to response headers.

Build and push it using: `REPO=${registry}/ext-proc-api-version make -C basic-sink docker-local`
