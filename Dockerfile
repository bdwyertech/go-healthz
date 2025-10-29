FROM centos/systemd@sha256:09db0255d215ca33710cc42e1a91b9002637eeef71322ca641947e65b7d53b58

RUN curl -sSL https://dl.google.com/go/go1.16.4.linux-amd64.tar.gz | tar zxv -C /opt

RUN ln -s /opt/go/bin/{go,gofmt} /usr/local/bin

WORKDIR /code
