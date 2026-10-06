# Disposable LOCAL test runner only; never a production validator container.
FROM python:3.6.8-alpine AS python368
FROM python:3.6-alpine
RUN apk add --no-cache openssl bash git libpq docker-cli && \
    apk add --no-cache --virtual .build gcc musl-dev postgresql-dev && \
    pip install --no-cache-dir psycopg2==2.9.5 && apk del .build && \
    mkdir -p /usr/local/lib/docker/cli-plugins && \
    wget -q https://github.com/docker/compose/releases/download/v2.35.1/docker-compose-linux-x86_64 -O /usr/local/lib/docker/cli-plugins/docker-compose && \
    chmod 755 /usr/local/lib/docker/cli-plugins/docker-compose
# Keep current fixture tooling/libpq but execute the exact required interpreter.
COPY --from=python368 /usr/local/bin/python3.6 /usr/local/bin/python3.6
COPY --from=python368 /usr/local/lib/libpython3.6m.so.1.0 /usr/local/lib/libpython3.6m.so.1.0
COPY --from=python368 /usr/local/lib/python3.6 /usr/local/lib/python3.6
ENV PYTHONDONTWRITEBYTECODE=1 ISSUE203_OWNED_APPROVAL_CONTAINER=1
WORKDIR /source
