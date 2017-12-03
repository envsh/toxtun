VERSION         :=      $(shell cat ./VERSION)
# VERSION=`git describe --tags`
GITBRANCH=`git rev-parse --abbrev-ref HEAD`
BUILDDATE=`date +%FT%T%z`
GOVVV=`govvv -flags -version ${VERSION}|sed 's/=/=GOVVV-/g'`

all: static dynamic

dynamic:
	echo ${GOVVV}
	go build -v -ldflags "${GOVVV}" .
	mv toxtun-go toxtun

static: export CGO_CFLAGS=-I$(PWD)/build/include/tox
static: export CGO_LDFLAGS=-L$(PWD)/build/lib
static: static_libsodium static_libopus static_libvpx static_libtoxcore
	# rm $(GOPATH)/pkg/linux_amd64/tox.a
	go install -v -x tox
	go build -v -ldflags "${GOVVV}" .
	mv toxtun-go toxtun-static

# 静态编译libtoxcore与其依赖库
static_libsodium:
	git clone https://github.com/jedisct1/libsodium.git || \
		cd libsodium/ && git checkout master && git pull
	cd libsodium/ && git checkout 'tags/1.0.3' && ./autogen.sh && \
		./configure --prefix=$(PWD)/build --disable-shared && \
		make -j3 > /dev/null && \
		make install > /dev/null

static_libopus:
	wget -c http://downloads.xiph.org/releases/opus/opus-1.1.tar.gz > /dev/null
	tar xzf opus-1.1.tar.gz > /dev/null
	cd opus-1.1 && ./configure --prefix=$(PWD)/build --disable-shared && \
		make -j3 > /dev/null && \
		make install > /dev/null

static_libvpx:
	git clone https://chromium.googlesource.com/webm/libvpx > /dev/null || \
		cd libvpx/ && git pull
	cd libvpx/ && ./configure --prefix=$(PWD)/build --disable-shared > /dev/null && \
		make -j3 >/dev/null && \
		make install > /dev/null

static_libtoxcore: export CFLAGS=-I$(PWD)/build/include/tox
static_libtoxcore: export LDFLAGS=-L$(PWD)/build/lib
static_libtoxcore:
	git clone https://github.com/irungentoo/toxcore.git || \
		cd toxcore/ && git pull
	cd toxcore/ && ./configure --prefix=$(PWD)/build --disable-shared --disable-tests --disable-daemon --disable-ntox && \
		make -j3 > /dev/null && \
		make install > /dev/null

