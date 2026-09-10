FROM grafana/k6:2.2.0

ENV K6_NO_USAGE_REPORT=true

COPY tools/proxy-bench/proxy-bench.js /home/k6/proxy-bench.js

ENTRYPOINT ["k6", "run", "/home/k6/proxy-bench.js"]
