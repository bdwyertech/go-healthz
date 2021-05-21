FROM centos/systemd

RUN curl -sSL https://dl.google.com/go/go1.16.4.linux-amd64.tar.gz | tar zxv -C /opt

RUN ln -s /opt/go/bin/{go,gofmt} /usr/local/bin

WORKDIR /code
