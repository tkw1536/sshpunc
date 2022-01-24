#!/bin/sh

echo "[$(date +"%Y-%m-%dT%H:%M:%S%z")] sshpunc starting"

echo "SSHHOST=$SSHHOST"
echo "SSHKEY=$SSHKEY"
echo "LOCALADDR=$LOCALADDR"
echo "REMOTEADDR=$LOCALADDR"

while true; do
    echo "[$(date +"%Y-%m-%dT%H:%M:%S%z")] Establishing new connection"
    ssh -o "UserKnownHostsFile=/dev/null" -o "ServerAliveInterval=60" -o "IdentitiesOnly=yes" -o "StrictHostKeyChecking=no" -o "IdentityFile=$SSHKEY" -L $LOCALADDR:$REMOTEADDR -N $SSHHOST
    echo "[$(date +"%Y-%m-%dT%H:%M:%S%z")] Connection lost"
    sleep 1
done
