# Static Go binary (WSDL embedded) on an empty base image (~8 MB instead of ~230 MB with Node).
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
COPY wsdl ./wsdl
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /loan-eligibility-soap .

FROM scratch
COPY --from=build /loan-eligibility-soap /loan-eligibility-soap
USER 65534:65534
EXPOSE 8088
HEALTHCHECK --interval=10s --timeout=3s --retries=5 CMD ["/loan-eligibility-soap", "-healthcheck"]
ENTRYPOINT ["/loan-eligibility-soap"]
