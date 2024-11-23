FROM busybox:1.37.0@sha256:db142d433cdde11f10ae479dbf92f3b13d693fd1c91053da9979728cceb1dc68

# makes /chimera/a.txt /chimera/b.txt
COPY . /

# make a layer where the dir is empty
RUN rm /chimera/a.txt /chimera/b.txt

RUN rmdir /chimera

RUN touch /chimera

RUN rm /chimera

RUN ln -s /etc/hostname /chimera

RUN unlink /chimera

RUN mkdir /chimera

