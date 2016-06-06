
FROM ubuntu

RUN echo "1"

RUN apt-get -y --force-yes update

RUN apt-get install -y --force-yes \
    cmake \
    make \
    libboost-all-dev \
    libxml2-dev \
    libxml++2.6-dev \
    libsqlite3-dev \
    libhdf5-serial-dev \
    libbz2-dev \
    coinor-libcbc-dev \
    coinor-libcoinutils-dev \
    coinor-libosi-dev \
    coinor-libclp-dev \
    coinor-libcgl-dev \
    libblas-dev \
    liblapack-dev \
    g++ \
    libgoogle-perftools-dev \
    git \
    python \
    python-tables \
    python-numpy \
    python-nose

RUN apt-get install -y --force-yes wget

ENV version=develop
# install cyclus and cycamore
RUN wget "https://github.com/cyclus/cyclus/archive/$version.tar.gz" -O "cyclus-$version.tar.gz"
RUN tar -xzf "cyclus-$version.tar.gz" && mkdir -p "cyclus-$version/Release"
WORKDIR cyclus-$version/Release
RUN cmake .. -DCMAKE_BUILD_TYPE=Release
RUN make && make install
WORKDIR /

RUN wget "https://github.com/rwcarlsen/cycamore/archive/$version.tar.gz" -O "cycamore-$version.tar.gz"
RUN tar -xzf "cycamore-$version.tar.gz" && mkdir -p "cycamore-$version/Release"
WORKDIR cycamore-$version/Release
RUN cmake .. -DCMAKE_BUILD_TYPE=Release
RUN make && make install
WORKDIR /

# bump number below to force update cloudlus
RUN echo "1"

RUN wget https://storage.googleapis.com/golang/go1.6.2.linux-amd64.tar.gz
RUN echo "export GOPATH=/usr/local" >> .profile
RUN echo "export PATH=$PATH:/usr/local/go/bin" >> .profile
ENV PATH $PATH:/usr/local/go/bin
ENV GOPATH /usr/local
RUN tar -C /usr/local -xzf go1.6.2.linux-amd64.tar.gz
RUN go version
RUN go get github.com/rwcarlsen/cloudlus/...
RUN go get github.com/rwcarlsen/cyan/cmd/cyan

ENTRYPOINT ["/bin/cloudlus", "-addr", "cycrun.fuelcycle.org:4343", "work", "-interval", "3s", "-whitelist", "cyclus,cyan,cycobj", "-timeout=3m"]
