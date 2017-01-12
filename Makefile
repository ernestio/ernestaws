test:
	echo "Not implemented... yet :("

lint:
	gometalinter --config .linter.conf

deps:
	echo "Not implemented... yet :("

dev-deps:
	go get github.com/alecthomas/gometalinter
	gometalinter --install
