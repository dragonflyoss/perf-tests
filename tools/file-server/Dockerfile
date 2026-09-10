FROM nginx:alpine

RUN touch /usr/share/nginx/html/zero

RUN dd if=/dev/zero of=/usr/share/nginx/html/nano bs=1 count=1

RUN dd if=/dev/zero of=/usr/share/nginx/html/micro bs=1K count=10

RUN dd if=/dev/zero of=/usr/share/nginx/html/small bs=1M count=1

RUN dd if=/dev/zero of=/usr/share/nginx/html/medium bs=1M count=10

RUN dd if=/dev/zero of=/usr/share/nginx/html/large bs=1G count=1

RUN dd if=/dev/zero of=/usr/share/nginx/html/xlarge bs=1G count=10

RUN dd if=/dev/zero of=/usr/share/nginx/html/xxlarge bs=1G count=30

RUN dd if=/dev/zero of=/usr/share/nginx/html/2g bs=1G count=2

RUN dd if=/dev/zero of=/usr/share/nginx/html/4g bs=1G count=4

RUN dd if=/dev/zero of=/usr/share/nginx/html/20g bs=1G count=20

RUN ln -s large /usr/share/nginx/html/1g && ln -s xlarge /usr/share/nginx/html/10g

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
