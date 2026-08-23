FROM node:24-alpine AS admin-build
WORKDIR /src/admin-vue
COPY admin-vue/package.json admin-vue/package-lock.json ./
RUN npm ci
COPY tsconfig.package.base.json /src/tsconfig.package.base.json
COPY packages /src/packages
COPY admin-vue ./
RUN npm run build

FROM node:24-alpine AS user-h5-build
WORKDIR /src/apps/user-uni
COPY apps/user-uni/package.json apps/user-uni/package-lock.json ./
RUN npm ci
COPY tsconfig.package.base.json /src/tsconfig.package.base.json
COPY packages /src/packages
COPY apps/user-uni ./
ENV VITE_API_BASE_URL=
RUN npm run build:h5

FROM golang:1.25-alpine AS api-build
WORKDIR /src/backend-go
COPY backend-go/go.mod backend-go/go.sum ./
ENV GOPROXY=https://goproxy.cn,direct
RUN for i in 1 2 3 4 5; do go mod download && exit 0; echo "go mod download failed, retrying in 5s ($i/5)"; sleep 5; done; go mod download
COPY backend-go ./
RUN go build -o /out/xianzhi-api ./cmd/api
RUN go build -o /out/smartvideo-worker ./cmd/smartvideo-worker

FROM alpine:3.20
WORKDIR /app
ARG INSTALL_SEEDANCE_SDK=false
ARG INSTALL_OFFICECLI=false
ARG ALPINE_MIRROR=
ARG OFFICECLI_INSTALL_URL=https://raw.githubusercontent.com/iOfficeAI/OfficeCLI/main/install.sh
ARG OFFICECLI_INSTALL_SHA256=
RUN if [ -n "$ALPINE_MIRROR" ]; then \
    sed -i "s#https://dl-cdn.alpinelinux.org/alpine#$ALPINE_MIRROR#g" /etc/apk/repositories; \
  fi \
  && apk add --no-cache ca-certificates curl bash icu-libs tzdata python3 py3-pip ffmpeg=6.1.1-r8 font-noto-cjk \
  && mkdir -p /app/seedance-python \
  && if [ "$INSTALL_SEEDANCE_SDK" = "true" ]; then \
    curl -fL --retry 5 --retry-delay 5 --retry-all-errors --connect-timeout 20 --max-time 180 "https://ecloud.10086.cn/api/query/maas/public/backend/model/link/aicc-sdk/python/download" -o /tmp/maas-seedance-sdk.zip \
    && python3 -c "import zipfile; zipfile.ZipFile('/tmp/maas-seedance-sdk.zip').extract('pythonSDK-0515/maas_seedance_sdk-1.0.0-py3-none-any.whl', '/tmp')" \
    && python3 -m pip install --break-system-packages --no-cache-dir --target /app/seedance-python /tmp/pythonSDK-0515/maas_seedance_sdk-1.0.0-py3-none-any.whl; \
  else \
    echo "Skipping optional Seedance SDK install"; \
  fi \
  && if [ "$INSTALL_OFFICECLI" = "true" ]; then \
    if [ -z "$OFFICECLI_INSTALL_SHA256" ]; then echo "INSTALL_OFFICECLI=true requires OFFICECLI_INSTALL_SHA256"; exit 1; fi \
    && curl -fsSL "$OFFICECLI_INSTALL_URL" -o /tmp/officecli-install.sh \
    && echo "$OFFICECLI_INSTALL_SHA256  /tmp/officecli-install.sh" | sha256sum -c - \
    && bash /tmp/officecli-install.sh \
    && cp /root/.local/bin/officecli /usr/local/bin/officecli \
    && chmod +x /usr/local/bin/officecli \
    && officecli --version; \
  else \
    echo "Skipping OfficeCLI install"; \
  fi
COPY --from=api-build /out/xianzhi-api /app/xianzhi-api
COPY --from=api-build /out/smartvideo-worker /app/smartvideo-worker
COPY --from=admin-build /src/admin-vue/dist /app/admin-vue/dist
COPY --from=user-h5-build /src/apps/user-uni/dist/build/h5 /app/user-h5
COPY backend-go/internal/provider/video/seedance_bridge.py /app/seedance_bridge.py
ENV PORT=3100
ENV XIANZHI_DATA_PATH=/app/data/store.json
ENV XIANZHI_STATIC_DIR=/app/admin-vue/dist
ENV XIANZHI_ADMIN_STATIC_DIR=/app/admin-vue/dist
ENV XIANZHI_USER_H5_STATIC_DIR=/app/user-h5
ENV CME_SEEDANCE_BRIDGE=/app/seedance_bridge.py
ENV CME_SEEDANCE_DEPS_PATH=/app/seedance-python
RUN mkdir -p /app/data /tmp/smartvideo \
  && printf '{"generationTasks":[],"assets":[],"counters":{}}\n' > /app/data/store.json \
  && adduser -D -H xianzhi \
  && chown -R xianzhi:xianzhi /app/data /tmp/smartvideo
USER xianzhi
EXPOSE 3100
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=5 \
  CMD curl -fsS http://127.0.0.1:3100/api/v1/health >/dev/null || exit 1
CMD ["/app/xianzhi-api"]
