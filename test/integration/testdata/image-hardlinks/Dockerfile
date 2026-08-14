FROM busybox:1.36@sha256:023917ec6a886d0e8e15f28fb543515a5fcd8d938edb091e8147db4efed388ee

# Hardlink scenarios, one case per directory. Each RUN is its own layer, so the layer boundaries
# below are meaningful: several cases depend on a later layer touching a path an earlier layer
# hardlinked.
#
# Two properties of the tar that `docker build` produces are load-bearing here, and both are easy to
# guess wrong:
#
#   - The data section goes to the ALPHABETICALLY FIRST name, not to the name created first. In c1
#     below, `orig.txt` is created and `hard.txt` is the `ln`, yet the tar carries the bytes on
#     `hard.txt` (REG) and emits `orig.txt` as the hardlink header, because the tar writer walks the
#     directory sorted.
#   - A hardlink header's target is always in the same layer, and always precedes it. `ln` across
#     layers forces an overlayfs copy-up, so it emits a full regular-file copy instead (see c2).

# c1: hardlink pair in one layer, then the OTHER name is modified in place in a later layer.
# overlayfs copies the modified name up to a new inode, so the two names diverge: hard.txt keeps the
# original contents.
RUN mkdir -p /c1 \
 && printf 'CASE1-ORIGINAL' > /c1/orig.txt \
 && ln /c1/orig.txt /c1/hard.txt

# c2: `ln` to a file from a PREVIOUS layer. this cannot produce a hardlink header; the layer diff
# contains a full copy of the contents.
RUN mkdir -p /c2 \
 && ln /c1/orig.txt /c2/cross-hard.txt

# c3: c1's other name modified in place, now that c2 has taken its copy of the original contents.
RUN printf 'CASE3-MODIFIED-IN-PLACE' > /c1/orig.txt

# c4: hardlink pair, then the hardlink-header name is replaced (rm + create) in a later layer.
RUN mkdir -p /c4 \
 && printf 'CASE4-ORIGINAL' > /c4/orig.txt \
 && ln /c4/orig.txt /c4/hard.txt
RUN rm /c4/orig.txt \
 && printf 'CASE4-REPLACED' > /c4/orig.txt

# c5: the shape produced by package managers such as uv and pip. an installed file is hardlinked
# into a content-addressed cache, then a later layer replaces the installed name with a symlink
# aimed at the cache name. squashed, the two names refer to each other, which reads as a link cycle
# even though the cache name still refers to the original inode on disk.
RUN mkdir -p /c5/bin /c5/cache \
 && printf 'CASE5-PAYLOAD' > /c5/bin/tool \
 && chmod +x /c5/bin/tool \
 && ln /c5/bin/tool /c5/cache/blob
RUN rm /c5/bin/tool \
 && ln -s /c5/cache/blob /c5/bin/tool

# c6: `ln` onto a SYMLINK. this hardlinks the symlink inode, so both names are symlinks to the same
# destination, and the tar emits a hardlink header whose target is a symlink member.
RUN mkdir -p /c6 \
 && printf 'CASE6-REAL' > /c6/real.txt \
 && ln -s /c6/real.txt /c6/sym.txt \
 && ln /c6/sym.txt /c6/hard-to-sym.txt

# c7: the hardlink-header name is deleted in a later layer. the data-carrying name survives.
RUN mkdir -p /c7 \
 && printf 'CASE7-PAYLOAD' > /c7/orig.txt \
 && ln /c7/orig.txt /c7/hard.txt
RUN rm /c7/orig.txt

# c8: three names for one inode, all in one layer: one regular header and two hardlink headers.
RUN mkdir -p /c8 \
 && printf 'CASE8-PAYLOAD' > /c8/a.txt \
 && ln /c8/a.txt /c8/b.txt \
 && ln /c8/a.txt /c8/c.txt

# c9: the DATA-CARRYING name (hard.txt, alphabetically first) is overwritten in a later layer. the
# other name still refers to the original inode.
RUN mkdir -p /c9 \
 && printf 'CASE9-ORIGINAL' > /c9/orig.txt \
 && ln /c9/orig.txt /c9/hard.txt
RUN printf 'CASE9-HARDLINK-OVERWRITTEN' > /c9/hard.txt

# c10: the data-carrying name is WHITEOUTED in a later layer, leaving a hardlink header whose
# linkname is absent from the squashed tree. the container still serves the file under z.txt:
# deleting one name of an inode does not remove the others.
RUN mkdir -p /c10 \
 && printf 'CASE10-PAYLOAD' > /c10/a.txt \
 && ln /c10/a.txt /c10/z.txt
RUN rm /c10/a.txt

# c11: the data-carrying name is replaced in a later layer, leaving a hardlink header pointing at a
# path whose contents have since changed.
RUN mkdir -p /c11 \
 && printf 'CASE11-PAYLOAD' > /c11/a.txt \
 && ln /c11/a.txt /c11/z.txt
RUN printf 'CASE11-A-REPLACED-LATER' > /c11/a.txt
