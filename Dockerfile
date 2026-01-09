FROM alpine:3.21

# Install Doppler CLI
RUN wget -q -t3 'https://packages.doppler.com/public/cli/rsa.8004D9FF50437357.key' -O /etc/apk/keys/cli@doppler-8004D9FF50437357.rsa.pub && \
    echo 'https://packages.doppler.com/public/cli/alpine/any-version/main' | tee -a /etc/apk/repositories && \
    apk add --no-cache doppler ca-certificates

# Install Node.js for Adyen MCP
RUN apk add --no-cache nodejs npm

WORKDIR /app

COPY bin/processor/bootstrap /app/bootstrap
COPY scripts/entrypoint.sh /app/entrypoint.sh

RUN chmod +x /app/entrypoint.sh

USER 321

ENTRYPOINT ["/app/entrypoint.sh"]
CMD ["/app/bootstrap"]
