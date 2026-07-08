# TODO(security-hygiene): pin ghcr.io/penguincloud/core to a sha256 digest.
# Digest lookup failed at edit time (internal/private image, no accessible `latest` tag).
# Retrieve with `docker buildx imagetools inspect ghcr.io/penguincloud/core:<tag>` and pin.
FROM ghcr.io/penguincloud/core AS BUILD
LABEL company="Penguin Tech Group LLC"
LABEL org.opencontainers.image.authors="info@penguintech.group"
LABEL license="GNU AGPL3"

# GET THE FILES WHERE WE NEED THEM!
COPY . /opt/manager/
WORKDIR /opt/manager


# PUT YER ARGS in here
ARG APP_TITLE="PTGAPP" # Change this to actual title for Default

# BUILD IT!
RUN ansible-playbook build.yml -c local --tags=build

# PUT YER ENVS in here
ENV HELLO="WORLD"

# Switch to non-root user, if you change run-user variable update this one too
USER ptg-user

# Entrypoint time (aka runtime)
ENTRYPOINT ["/bin/bash","/opt/manager/entrypoint.sh"]
