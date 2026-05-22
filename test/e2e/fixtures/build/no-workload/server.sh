#!/bin/sh
# Trivial HTTP responder used by the no-workload e2e fixture. busybox's httpd
# serves the working directory; we just need any 2xx on /.
echo "no-workload e2e ok" > /tmp/index.html
exec httpd -f -p 8080 -h /tmp
