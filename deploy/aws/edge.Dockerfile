FROM nginx@sha256:abe47724e466aeab9a345d8e46a221c2fa8953c7848bb4a3bd9976a7199f8cf2
COPY deploy/aws/nginx.conf /etc/nginx/nginx.conf
COPY frontend/build/web/ /usr/share/nginx/html/
ENTRYPOINT ["nginx", "-g", "daemon off;"]
