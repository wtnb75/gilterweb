FROM scratch
COPY gilterweb /gilterweb
ENTRYPOINT ["/gilterweb"]
