FROM nginx:alpine

RUN dd if=/dev/zero of=/usr/share/nginx/html/nano bs=1 count=1

RUN dd if=/dev/zero of=/usr/share/nginx/html/micro bs=1K count=10

RUN dd if=/dev/zero of=/usr/share/nginx/html/small bs=1M count=1

RUN dd if=/dev/zero of=/usr/share/nginx/html/medium bs=1M count=10

RUN dd if=/dev/zero of=/usr/share/nginx/html/large bs=1G count=1

RUN dd if=/dev/zero of=/usr/share/nginx/html/xlarge bs=1G count=10

RUN dd if=/dev/zero of=/usr/share/nginx/html/xxlarge bs=1G count=30

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
