# syntax=docker/dockerfile:1
# check=skip=SecretsUsedInArgOrEnv
FROM jzaplet/documan:2.15.0

COPY ./docs /documan/data/docs
COPY ./README.md /documan/data/README.md

WORKDIR /documan/data

# Configure your project
ENV DOCUMAN_PROJECT_NAME='GO Kick'
ENV DOCUMAN_DOCS_FILES='**/*.md'
ENV DOCUMAN_EXCLUDED_FILES=''
ENV DOCUMAN_HTTP_PORT=3000
ENV DOCUMAN_OPENAI_EMBEDDING_MODEL='text-embedding-3-small'
ENV DOCUMAN_CHUNK_MAX_LEN=250

ARG DOCUMAN_LICENSE_KEY=''
ARG DOCUMAN_OPENAI_API_KEY=''

# Set to "true" to skip lint, import, and vectorize during build.
# Useful for first-time setup when docs may not have valid frontmatter yet.
ARG SKIP_BUILD_STEPS=false

# Validate markdown files — checks for broken links, invalid frontmatter, etc.
# Fails the build if any issues are found
RUN if [ "$SKIP_BUILD_STEPS" = "false" ]; then /documan/bin/documan lint; fi

# Import docs (license key optional — needed for >100 files)
# Uses Docker secret if available, otherwise falls back to ARG
RUN --mount=type=secret,id=DOCUMAN_LICENSE_KEY,required=false \
    if [ "$SKIP_BUILD_STEPS" = "true" ]; then exit 0; fi && \
    if [ -f /run/secrets/DOCUMAN_LICENSE_KEY ]; then \
      export DOCUMAN_LICENSE_KEY=$(cat /run/secrets/DOCUMAN_LICENSE_KEY); \
    fi && \
    /documan/bin/documan import

# Vectorize docs (only runs if OpenAI API key is provided)
# Uses Docker secret if available, otherwise falls back to ARG
RUN --mount=type=secret,id=DOCUMAN_OPENAI_API_KEY,required=false \
    if [ "$SKIP_BUILD_STEPS" = "true" ]; then exit 0; fi && \
    if [ -f /run/secrets/DOCUMAN_OPENAI_API_KEY ]; then \
      export DOCUMAN_OPENAI_API_KEY=$(cat /run/secrets/DOCUMAN_OPENAI_API_KEY); \
    fi && \
    if [ -n "${DOCUMAN_OPENAI_API_KEY}" ]; then /documan/bin/documan vectorize; fi

CMD ["/documan/bin/documan", "serve"]