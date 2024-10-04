
.PHONY: all clean fmt

all:
	./build -s

fmt:
	gofmt -w -s src/*.go internal/*/*.go

clean:
	-rm -rf ./bin
