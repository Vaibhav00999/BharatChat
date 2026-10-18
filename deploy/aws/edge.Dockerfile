FROM nginx@sha256:332d0beea97cff8c0d5d27976c0778a4b00345b9d74c2feaaf8efde548314948
COPY deploy/aws/nginx.conf /etc/nginx/nginx.conf
COPY frontend/build/web/ /usr/share/nginx/html/
ENTRYPOINT ["nginx", "-g", "daemon off;"]
