FROM golang:1.9.1 as builder
LABEL builder=true
ENV srcDir="/go/src/github.com/yglcode/kafka-sarama-pingpong"
RUN mkdir -p $srcDir
COPY . $srcDir/
WORKDIR $srcDir
RUN CGO_ENABLED=0 go build -a --installsuffix cgo --ldflags="-s" -o pingpong

FROM scratch
ENV srcDir="/go/src/github.com/yglcode/kafka-sarama-pingpong"
COPY --from=builder $srcDir/pingpong .
CMD ["/pingpong"]
