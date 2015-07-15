
FROM base/archlinux

RUN echo "1"

# this trusts all keys and allows the system update to run
RUN mv /etc/pacman.conf /etc/pacman.conf.back
RUN sed 's/^SigLevel *= .*/SigLevel = TrustAll/' /etc/pacman.conf.back > /etc/pacman.conf

RUN pacman -Syu --noconfirm
RUN pacman-db-upgrade

# developer packages
RUN pacman -S --noconfirm base-devel
RUN pacman -S --noconfirm cmake
RUN pacman -S --noconfirm git
RUN pacman -S --noconfirm mercurial
RUN pacman -S --noconfirm abs
RUN pacman -S --noconfirm gcc-fortran
RUN pacman -S --noconfirm go
RUN pacman -S --noconfirm wget

# cyclus dependencies
RUN pacman -S --noconfirm sqlite
RUN pacman -S --noconfirm boost
RUN pacman -S --noconfirm libxml++
RUN pacman -S --noconfirm hdf5
RUN pacman -S --noconfirm python2
RUN pacman -S --noconfirm python2-nose
RUN pacman -S --noconfirm python2-pytables
RUN pacman -S --noconfirm coin-or-cbc
RUN ln -s /usr/bin/python2 /usr/local/bin/python

ENV version=develop
# install cyclus and cycamore
RUN wget "https://github.com/cyclus/cyclus/archive/$version.tar.gz" -O "cyclus-$version.tar.gz"
RUN tar -xzf "cyclus-$version.tar.gz" && mkdir -p "cyclus-$version/Release"
WORKDIR cyclus-$version/Release
RUN cmake .. -DCMAKE_BUILD_TYPE=Release
RUN make && make install
WORKDIR /

RUN wget "https://github.com/cyclus/cycamore/archive/$version.tar.gz" -O "cycamore-$version.tar.gz"
RUN tar -xzf "cycamore-$version.tar.gz" && mkdir -p "cycamore-$version/Release"
WORKDIR cycamore-$version/Release
RUN cmake .. -DCMAKE_BUILD_TYPE=Release
RUN make && make install
WORKDIR /

# bump number below to force update cloudlus
RUN echo "1"

ENV GOPATH /
RUN go get github.com/rwcarlsen/cloudlus/...
RUN go get github.com/rwcarlsen/cyan/cmd/cyan

ENTRYPOINT ["/bin/cloudlus", "-addr", "cycrun.fuelcycle.org:80", "work", "-interval", "3s", "-whitelist", "cyclus", "cyan", "-timeout=3m"]
