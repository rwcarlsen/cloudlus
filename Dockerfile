
FROM base/archlinux

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
RUN git clone https://github.com/rwcarlsen/arch-abs

RUN pacman -S --noconfirm --asdeps lapack blas 
# can't run makepkg as root anymore
USER nobody
RUN cp -R arch-abs/coin-cbc /tmp/
RUN cd /tmp/coin-cbc && makepkg
USER root
RUN pacman -U --noconfirm /tmp/coin-cbc/coin-cbc*.xz

RUN pacman -S --noconfirm sqlite
RUN pacman -S --noconfirm boost
RUN pacman -S --noconfirm libxml++
RUN pacman -S --noconfirm hdf5
RUN pacman -S --noconfirm python2
RUN pacman -S --noconfirm python2-nose
RUN pacman -S --noconfirm python2-pytables
RUN ln -s /usr/bin/python2 /usr/local/bin/python

ENV version=1.2.0
# install cyclus and cycamore
RUN wget "https://github.com/cyclus/cyclus/archive/$version.tar.gz" -O "cyclus-$version.tar.gz"
RUN tar -xzf "cyclus-$version.tar.gz" && mkdir -p "cyclus-$version/Release"
WORKDIR cyclus-$version/Release
RUN cmake .. -DCMAKE_BUILD_TYPE=Release
RUN make && make install
WORKDIR /

ENV version=1.2.0
RUN wget "https://github.com/rwcarlsen/cycamore/archive/$version.tar.gz" -O "cycamore-$version.tar.gz"
RUN tar -xzf "cycamore-$version.tar.gz" && mkdir -p "cycamore-$version/Release"
WORKDIR cycamore-$version/Release
RUN cmake .. -DCMAKE_BUILD_TYPE=Release
RUN make && make install
WORKDIR /

# bump number below to force update cloudlus
RUN echo "1"

ENV GOPATH /
RUN go get github.com/rwcarlsen/cloudlus/...
RUN go get github.com/rwcarlsen/cyan/...

ENTRYPOINT ["/bin/cloudlus", "-addr", "cycrun.fuelcycle.org:80", "work", "-interval", "3s", "-whitelist", "cyclus"]
