all: static dynamic

dynamic:
	go build -v .
	mv toxtun-go toxtun

# 静态编译libtoxcore与其依赖库
static:
	go build -v .
	mv toxtun-go toxtun-static

