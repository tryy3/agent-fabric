# syntax=docker/dockerfile:1
# Build context: repository root.

# Cirrus Labs stopped publishing after 3.44; Project516 continues the same tags.
FROM ghcr.io/project516/flutter:3.47.0 AS build
WORKDIR /app
COPY client/pubspec.yaml client/pubspec.lock ./
RUN flutter pub get
COPY client/ ./
RUN flutter build web --release --no-wasm-dry-run

FROM nginx:1.27-alpine
COPY --from=build /app/build/web /usr/share/nginx/html
COPY client/docker/nginx.conf.template /etc/nginx/templates/default.conf.template
COPY client/docker/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
ENV CONTROLPLANE_UPSTREAM=http://controlplane:8080
EXPOSE 80
ENTRYPOINT ["/entrypoint.sh"]
