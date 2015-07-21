#!/bin/zsh
mkdir -p /home/stapelberg/public_html/dh-make-golang
TEMPFILE=$(mktemp)
wget -q -O- http://httpredir.debian.org/debian/dists/sid/main/binary-amd64/Packages.gz | zgrep '^Package: golang-' | uniq > "${TEMPFILE}"
if [ -s "${TEMPFILE}" ]; then
  chmod o+r "${TEMPFILE}"
  mv "${TEMPFILE}" /home/stapelberg/public_html/dh-make-golang/binary-amd64-grep-golang
else
  rm "${TEMPFILE}"
fi
