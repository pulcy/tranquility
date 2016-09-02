FROM alpine:3.4

ADD ./tranquility /app/

ENTRYPOINT ["/app/tranquility"]
