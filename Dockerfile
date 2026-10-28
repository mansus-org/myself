FROM busybox:stable-musl
COPY site /www
USER 10001:10001
EXPOSE 8080
# -f foreground, -v log requests to stderr, no config needed.
CMD ["httpd", "-f", "-v", "-p", "8080", "-h", "/www"]
