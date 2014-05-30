
FROM base/arch

RUN pacman -Syu --noconfirm

# developer packages
RUN pacman -S --noconfirm base-devel
RUN pacman -S --noconfirm cmake
RUN pacman -S --noconfirm git
RUN pacman -S --noconfirm mercurial
RUN pacman -S --noconfirm abs
RUN pacman -S --noconfirm gcc-fortran
RUN pacman -S --noconfirm go

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
RUN wget https://github.com/cyclus/cyclus/archive/1.0.0-rc4.tar.gz -O cyclus-1.0.0-rc4.tar.gz
RUN tar -xzf cyclus-1.0.0-rc4.tar.gz
RUN cd cyclus-1.0.0-rc4 && mkdir release && cd release && cmake .. -DCMAKE_BUILD_TYPE=Release && make && make install

RUN wget https://github.com/cyclus/cycamore/archive/1.0.0-rc4.tar.gz -O cycamore-1.0.0-rc4.tar.gz
RUN tar -xzf cycamore-1.0.0-rc4.tar.gz
RUN cd cycamore-1.0.0-rc4 && mkdir release && cd release && cmake .. -DCMAKE_BUILD_TYPE=Release && make && make install

# install other modules
#RUN git clone https://github.com/cyclus/kitlus && cd kitlus/kitlus && PREFIX=/usr/local make install
#RUN git clone https://github.com/rwcarlsen/transoptim && cd transoptim/agents && PREFIX=/usr/local make install

ENV GOPATH /
RUN go get github.com/rwcarlsen/cloudlus
RUN go get github.com/rwcarlsen/cyan/...
