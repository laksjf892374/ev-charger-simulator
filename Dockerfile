FROM golang:1.26-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /cposim .

# distroless/static carries CA certificates, which pushing to a real eMSP over https needs
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /cposim /cposim
ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/cposim"]
