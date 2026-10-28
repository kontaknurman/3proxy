# 3proxy, built from this repository, and 3proxy-panel in one image. Build it
# from the repository root:
#
#   docker build -f panel/Dockerfile -t 3proxy-panel .
#
# or with panel/docker-compose.yml, see panel/README.md.

FROM docker.io/alpine:3.23 AS proxy
RUN apk add --no-cache gcc make musl-dev linux-headers
WORKDIR /src
COPY Makefile.Linux Makefile.inc ./
COPY src ./src
RUN mkdir -p bin && make -f Makefile.Linux -j"$(nproc)" PAM_CHECK=false && strip bin/3proxy

FROM docker.io/golang:1.24-alpine AS panel
WORKDIR /src
COPY panel/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /3proxy-panel .

FROM docker.io/alpine:3.23
# tini reaps the processes of the 3proxy restart loop.
RUN apk add --no-cache tini
COPY --from=proxy /src/bin/3proxy /usr/local/bin/3proxy
COPY --from=panel /3proxy-panel /usr/local/bin/3proxy-panel
COPY --chmod=0755 panel/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
# 3proxy runs as root inside the container without a chroot, so the
# configuration lives next to the data.
ENV PANEL_LISTEN=:8088 \
    PANEL_DATA=/data/proxies.json \
    PANEL_CONFIG=/data/3proxy.cfg \
    PANEL_CHROOT= \
    PANEL_RELOAD="pkill -USR1 -x 3proxy"
VOLUME /data
ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/docker-entrypoint.sh"]
