FROM alpine:3.15

# install ssh and bash
RUN apk add --no-cache \
    ca-certificates \
    openssh-client

ADD script.sh /script.sh

# put the ssh key in /data/id
VOLUME /data/id
ENV SSHKEY /data/id

# setup the local address
EXPOSE 8080
ENV LOCALADDR 0.0.0.0:8080

CMD "/bin/sh" "/script.sh"