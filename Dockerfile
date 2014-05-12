
FROM base/arch

RUN pacman -Syu --noconfirm base-devel

# developer packages
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
RUN git clone https://github.com/cyclus/cyclus
RUN cd cyclus && mkdir build && cd build && cmake .. && make && make install

RUN git clone https://github.com/cyclus/cycamore
RUN cd cycamore && mkdir build && cd build && cmake .. && make && make install

# install other modules
RUN git clone https://github.com/cyclus/kitlus && cd kitlus/kitlus && PREFIX=/usr/local make install
RUN git clone https://github.com/rwcarlsen/transoptim && cd transoptim/agents && PREFIX=/usr/local make install

ENV GOPATH /
RUN go get github.com/rwcarlsen/cloudlus
