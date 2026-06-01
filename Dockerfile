FROM golang:1.21-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o /app/api-bin cmd/api/main.go

FROM alpine:latest

# Certificados CA para chamadas HTTP externas (API MK Solutions)
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copia o binário compilado
COPY --from=builder /app/api-bin .

# Copia os templates HTML (necessários em runtime para renderização SSR)
COPY --from=builder /app/views ./views

# Cria diretório para o banco SQLite (será montado como volume)
RUN mkdir -p /app/data

EXPOSE 8080
CMD ["./api-bin"]
