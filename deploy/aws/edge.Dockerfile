FROM nginx@sha256:4c41e2d0a3a55b325ed572cd0200bdb72af1433c3daa2bca6ef8eea68ce504a2
COPY deploy/aws/nginx.conf /etc/nginx/nginx.conf
COPY frontend/build/web/ /usr/share/nginx/html/
ENTRYPOINT ["nginx", "-g", "daemon off;"]
