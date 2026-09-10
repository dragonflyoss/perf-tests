FROM nginx:alpine

# Files are named by size, the name is the URL path, e.g. http://file-server/1g.
RUN dd if=/dev/zero of=/usr/share/nginx/html/1b bs=1 count=1

RUN dd if=/dev/zero of=/usr/share/nginx/html/1k bs=1K count=1

RUN dd if=/dev/zero of=/usr/share/nginx/html/1m bs=1M count=1

RUN dd if=/dev/zero of=/usr/share/nginx/html/4m bs=1M count=4

RUN dd if=/dev/zero of=/usr/share/nginx/html/10m bs=1M count=10

RUN dd if=/dev/zero of=/usr/share/nginx/html/1g bs=1G count=1

RUN dd if=/dev/zero of=/usr/share/nginx/html/2g bs=1G count=2

RUN dd if=/dev/zero of=/usr/share/nginx/html/4g bs=1G count=4

RUN dd if=/dev/zero of=/usr/share/nginx/html/10g bs=1G count=10

RUN dd if=/dev/zero of=/usr/share/nginx/html/20g bs=1G count=20

RUN dd if=/dev/zero of=/usr/share/nginx/html/30g bs=1G count=30

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
