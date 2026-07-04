ARG BASE_IMAGE=ubuntu:24.04
FROM ${BASE_IMAGE}

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates curl sudo openssh-server iproute2 \
  && rm -rf /var/lib/apt/lists/* \
  && mkdir -p /run/systemd/system /etc/ssh/sshd_config.d

COPY dist/servy /usr/local/bin/servy
COPY examples /examples
RUN chmod 0755 /usr/local/bin/servy

RUN servy version
RUN servy validate --config /examples/base.yml
RUN servy validate --config /examples/docker-only.yml
RUN servy validate --config /examples/node.yml
RUN servy validate --config /examples/web-app.yml
RUN servy plan --config /examples/base.yml
RUN servy plan --config /examples/base.yml --json
RUN servy apply --config /examples/base.yml --dry-run
RUN servy apply --config /examples/docker-only.yml --dry-run
RUN servy apply --config /examples/node.yml --dry-run
RUN servy apply --config /examples/web-app.yml --dry-run
RUN servy module list
RUN servy doctor
RUN servy doctor --json
RUN servy init --list-presets
RUN servy init --preset docker-only --output /tmp/generated-servy.yml
RUN servy validate docker-only --config /tmp/generated-servy.yml
RUN servy status --config /examples/base.yml
