FROM golang:1.23-alpine AS build
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 go build -o gcs-mock-service

FROM alpine:3.20
WORKDIR /app
COPY --from=build /app/gcs-mock-service .
RUN chmod +x /app/gcs-mock-service

EXPOSE 4443
CMD ["./gcs-mock-service"]
