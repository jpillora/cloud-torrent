shopt -s nullglob
for a in "${TMPDIR:-/tmp}"/torrentfs*; do
	sudo umount -f "$a/mnt"
	rm -r -- "$a"
done
