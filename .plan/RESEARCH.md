# Research: Wave 4 Plan Review
> Generated: 2026-05-29

## Standard Stack
| Library | Version | Purpose | Provenance |
|---------|---------|---------|------------|
| gorilla/websocket | current | WebSocket upgrade + cookie access | [ASSUMED] |
| kubernetes/client-go | ~0.31 | K8s Job CRUD | [ASSUMED] |
| boto3 | current | S3 upload/download | [ASSUMED] |
| golang-jwt/jwt/v5 | v5 | RS256 JWT | [VERIFIED from codebase] |

## Architecture Pattern
Current: Celery worker calls `docker run` directly via subprocess. Wave 4H replaces this with the Kubernetes Python client creating `batch/v1` Jobs. [ASSUMED from plan description]

## Package Legitimacy
| Package | Registry Check | Result |
|---------|---------------|--------|
| kubernetes (Python) | pip index versions kubernetes | Not run - research agent |

## Open Questions
See numbered findings below.
