# # build the user permission server
# FROM alpine as permission
# 
# # Create www-data
# RUN set -x ; \
#   addgroup -g 82 -S www-data ; \
#   adduser -u 82 -D -S -G www-data www-data && exit 0 ; exit 1
# 
# build the server
FROM golang as build

# build the app
ADD . /app/
WORKDIR /app/
RUN go get -v ./...
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /app/sshproc .

# add it into a scratch image
FROM scratch
WORKDIR /

# add the user
# COPY --from=permission /etc/passwd /etc/passwd
# COPY --from=permission /etc/group /etc/group

# add the app
COPY --from=build /app/sshproc /sshproc

# put the ssh key in /data/id
ENV SSHKEY /data/id

# setup the local address
EXPOSE 8080
ENV LOCALADDR 0.0.0.0:8080

# USER www-data:www-data
CMD ["/sshproc"]
