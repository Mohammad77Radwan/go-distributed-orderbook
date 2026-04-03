# syntax=docker/dockerfile:1

FROM node:20-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.24-alpine AS backend-builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN go build -o quantum-engine ./main.go

FROM alpine:3.23
WORKDIR /app
RUN apk add --no-cache ca-certificates
COPY --from=backend-builder /app/quantum-engine ./quantum-engine
COPY --from=frontend-builder /app/frontend/build ./frontend/build
EXPOSE 8080
CMD ["./quantum-engine"]
