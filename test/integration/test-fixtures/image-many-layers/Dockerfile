FROM busybox:1.36@sha256:023917ec6a886d0e8e15f28fb543515a5fcd8d938edb091e8147db4efed388ee
ADD file.txt /somefile.txt
RUN mkdir -p /root/example/really/nested
RUN cp /somefile.txt /root/example/somefile1.txt
RUN chmod 444 /root/example/somefile1.txt
RUN cp /somefile.txt /root/example/somefile2.txt
RUN cp /somefile.txt /root/example/somefile3.txt
RUN mv /root/example/somefile3.txt /root/saved.txt
RUN cp /root/saved.txt /root/.saved.txt
RUN rm -rf /root/example/
ADD a-dir /root/.data/
RUN cp /root/saved.txt /tmp/saved.again1.txt
RUN cp /root/saved.txt /root/.data/saved.again2.txt
RUN chmod +x /root/saved.txt
RUN chmod 421 /root
