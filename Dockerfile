
FROM base/archlinux

RUN pacman -Syu --noconfirm

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
RUN cd arch-abs/coin-cbc && makepkg -si --asroot --noconfirm
RUN pacman -S --noconfirm sqlite
RUN pacman -S --noconfirm boost
RUN pacman -S --noconfirm libxml++
RUN pacman -S --noconfirm hdf5
RUN pacman -S --noconfirm python2
RUN pacman -S --noconfirm python2-nose
RUN pacman -S --noconfirm python2-pytables
RUN ln -s /usr/bin/python2 /usr/local/bin/python

# install cyclus and cycamore
RUN wget https://github.com/cyclus/cyclus/archive/v1.1.1.tar.gz -O cyclus-1.1.1.tar.gz
RUN tar -xzf cyclus-1.1.1.tar.gz && mkdir -p cyclus-1.1.1/Release
RUN cd cyclus-1.1.1/Release && cmake .. -DCMAKE_BUILD_TYPE=Release
RUN cd cyclus-1.1.1/Release && make && make install

RUN wget https://github.com/cyclus/cycamore/archive/v1.1.1.tar.gz -O cycamore-1.1.1.tar.gz
RUN tar -xzf cycamore-1.1.1.tar.gz && mkdir -p cycamore-1.1.1/Release
RUN cd cycamore-1.1.1/Release && cmake .. -DCMAKE_BUILD_TYPE=Release
RUN cd cycamore-1.1.1/Release && make && make install

# install other modules
RUN git clone https://github.com/cyclus/kitlus
RUN cd kitlus/kitlus && PREFIX=/usr/local make install
RUN cd kitlus/agents && PREFIX=/usr/local make install

# install aurget
RUN wget https://aur.archlinux.org/packages/au/aurget/aurget.tar.gz
RUN tar -xzf aurget.tar.gz
RUN cd aurget && makepkg -si --asroot --noconfirm

# install bright light
#RUN aurget -S --noconfirm --asroot cblas
#RUN git clone https://github.com/FlanFlanagan/Bright-lite
#RUN mkdir -p Bright-lite/Release
#RUN cd Bright-lite/Release && cmake .. -DCMAKE_BUILD_TYPE=Release
#RUN cd Bright-lite/Release && make && make install

# bump number below to force update cloudlus
RUN echo "1"

ENV GOPATH /
RUN go get github.com/rwcarlsen/cloudlus/...
RUN go get github.com/rwcarlsen/cyan/...

cloudlus -addr=cycrun.fuelcycle.org:80 work -interval=5s -whitelist=cyclus
ENTRYPOINT ["/bin/cloudlus", "-addr", "cycrun.fuelcycle.org:80", "work", "-interval", "3s", "-whitelist", "cyclus"]
