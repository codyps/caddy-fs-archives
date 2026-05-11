ARG version=2.11.2
FROM caddy:${version}-builder-alpine AS builder

RUN xcaddy build --with github.com/codyps/caddy-fs-archives

FROM caddy:${version}

COPY --from=builder /usr/bin/caddy /usr/bin/caddy
