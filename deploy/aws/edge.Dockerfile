FROM nginx@sha256:165ddde8751e75a1c907d24ef10d9f6deb08b57e0fe56caf67512486aff76d82
COPY deploy/aws/nginx.conf /etc/nginx/nginx.conf
COPY frontend/build/web/ /usr/share/nginx/html/
ENTRYPOINT ["nginx", "-g", "daemon off;"]
