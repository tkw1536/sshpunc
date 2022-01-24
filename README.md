# sshpunc -- Puncture firewall using docker

![CI Status](https://github.com/tkw1536/sshpunc/workflows/docker/badge.svg)

This repository contains a docker image that port forwards to a remote ssh server.
The code is licensed under the Unlicense, hence in the public domain. 

This is intended to be used inside of Docker, and can be found as [a GitHub Package](https://github.com/users/tkw1536/packages/container/package/sshpunc). 
To start it up run:

```
docker run --rm -ti -p 8080:8080 -v $(pwd)/id_rsa:/data/id_rsa:ro -e SSHHOST=username@server.com -e REMOTEADDR=internal.lan:80 ghcr.io/tkw1536/sshpunc
```
