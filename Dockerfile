FROM node:24-alpine AS web-build
WORKDIR /src/frontend-vue
COPY frontend-vue/package.json frontend-vue/package-lock.json ./
RUN npm ci
COPY frontend-vue ./
RUN npm run build

FROM node:24-alpine AS admin-build
WORKDIR /src/admin-vue
COPY admin-vue/package.json admin-vue/package-lock.json ./
RUN npm ci
COPY admin-vue ./
RUN npm run build

FROM golang:1.25-alpine AS api-build
WORKDIR /src/backend-go
COPY backend-go/go.mod backend-go/go.sum ./
RUN go mod download
COPY backend-go ./
RUN go build -o /out/xianzhi-api ./cmd/api

FROM alpine:3.20
WORKDIR /app
COPY --from=api-build /out/xianzhi-api /app/xianzhi-api
COPY --from=web-build /src/frontend-vue/dist/build/h5 /app/frontend-vue/dist
COPY --from=admin-build /src/admin-vue/dist /app/admin-vue/dist
ENV PORT=3100
ENV XIANZHI_DATA_PATH=/app/data/store.json
ENV XIANZHI_STATIC_DIR=/app/frontend-vue/dist
ENV XIANZHI_ADMIN_STATIC_DIR=/app/admin-vue/dist
RUN mkdir -p /app/data \
  && printf '{"generationTasks":[],"assets":[],"counters":{}}\n' > /app/data/store.json \
  && adduser -D -H xianzhi \
  && chown -R xianzhi:xianzhi /app
USER xianzhi
EXPOSE 3100
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=5 \
  CMD wget -qO- http://127.0.0.1:3100/api/v1/health >/dev/null || exit 1
CMD ["/app/xianzhi-api"]
