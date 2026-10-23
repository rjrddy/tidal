FROM node:22-bookworm-slim AS web
WORKDIR /web
COPY apps/web/package*.json ./
RUN npm ci
COPY apps/web/ ./
RUN npm run build
FROM golang:1.27.3-bookworm AS core
WORKDIR /src
COPY go.mod ./
COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=0 go build -trimpath -o /tidal ./cmd/tidal
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=core /tidal /app/tidal
COPY --from=web /web/dist /app/apps/web/dist
COPY data/ /app/data/
COPY docs/results/campaign.json /app/docs/results/campaign.json
EXPOSE 8095
CMD ["/app/tidal", "-addr", "0.0.0.0:8095"]
