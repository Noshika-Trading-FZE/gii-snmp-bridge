FROM scratch
MAINTAINER Vladimir Gordeev <cptpower@gmail.com>

ENV APP_DIR /opt/bridge
WORKDIR $APP_DIR

COPY ./build/bridge $APP_DIR/
COPY ./bridge.toml $APP_DIR/bridge.toml
COPY ./ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

CMD ["./bridge"]
