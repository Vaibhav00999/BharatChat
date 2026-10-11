FROM nginx@sha256:f9ea18bfa4fad859e1ed38259d711da7ccad2c3516e875cec3351a57c859f571
COPY deploy/aws/nginx.conf /etc/nginx/nginx.conf
COPY frontend/build/web/ /usr/share/nginx/html/
ENTRYPOINT ["nginx", "-g", "daemon off;"]
