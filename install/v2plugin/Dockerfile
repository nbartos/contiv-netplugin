# Docker v2plugin container with OVS / netplugin / netmaster

FROM alpine:3.5
LABEL maintainer "Cisco Contiv (https://contiv.github.io)"

RUN mkdir -p /run/docker/plugins /etc/openvswitch /var/run/contiv/log \
 && echo 'http://dl-cdn.alpinelinux.org/alpine/v3.4/main' >> /etc/apk/repositories \
 && apk --no-cache add openvswitch=2.5.0-r0 iptables ca-certificates openssl curl \
 && wget -q -O /etc/apk/keys/sgerrand.rsa.pub https://raw.githubusercontent.com/andyshinn/alpine-pkg-glibc/master/sgerrand.rsa.pub \
 && wget https://github.com/andyshinn/alpine-pkg-glibc/releases/download/2.23-r1/glibc-2.23-r1.apk \
 && apk --no-cache add glibc-2.23-r1.apk

ARG TAR_FILE
ADD ${TAR_FILE} /

# This container is never run, it is exported for docker to run as a plugin.
# The start-netplugin and start-netmaster scripts, as well as the netplugin
# binaries are copied into the plugin's rootfs after export.  This avoids
# copying into and out of container.
COPY netplugin netmaster netctl start-netplugin start-netmaster /usr/local/sbin/
