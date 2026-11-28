FROM grafana/k6:2.3.0

ENV K6_NO_USAGE_REPORT=true

COPY tools/proxy-bench/proxy-bench.js /home/k6/proxy-bench.js
COPY tools/proxy-bench/lib /home/k6/lib

ENTRYPOINT ["k6", "run", "/home/k6/proxy-bench.js"]
