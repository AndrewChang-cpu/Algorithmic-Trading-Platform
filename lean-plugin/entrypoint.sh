#!/bin/bash
set -e

# Download job inputs from S3 (config.json, algorithm/, data/)
aws s3 sync "s3://${S3_BUCKET}/jobs/${JOB_ID}/input/" /lean/

_shutdown() {
    echo "Shutdown signal received, stopping LEAN..."
    kill -TERM "$LEAN_PID" 2>/dev/null
    wait "$LEAN_PID" 2>/dev/null
    echo "Uploading results on shutdown..."
    aws s3 sync /lean/Results/ "s3://${S3_BUCKET}/jobs/${JOB_ID}/results/" || true
    exit 0
}

trap '_shutdown' SIGTERM SIGINT

# Run LEAN in background so the trap can fire on SIGTERM
/Lean/Launcher/bin/Debug/Lean.Launcher &
LEAN_PID=$!
wait "$LEAN_PID"
EXIT_CODE=$?

# Normal exit path: upload results
echo "LEAN exited with code ${EXIT_CODE}, uploading results..."
aws s3 sync /lean/Results/ "s3://${S3_BUCKET}/jobs/${JOB_ID}/results/" || true

exit $EXIT_CODE
