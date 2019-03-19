FROM debian

# todo: find a way better to copy config
#ADD pkg/knowdy/knowdy/etc/knowdy/shard.gsl /etc/knowdy/
ADD gnode /usr/bin/

ADD schemas /etc/knowdy/schemas
RUN ls /etc/knowdy/schemas

EXPOSE 8081
CMD ["gnode", "--listen-address=0.0.0.0:8081", "--config-path=/etc/knowdy/shard.gsl"]
