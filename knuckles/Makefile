all:
	cd .. && go install
	go build
	./knuckles -config=./knuckles.sample.conf

linux:
	cd .. && GOARCH=amd64 GOOS=linux go install
	GOARCH=amd64 GOOS=linux go build

clean:
	go clean
