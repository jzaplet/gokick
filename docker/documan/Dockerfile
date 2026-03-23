# syntax=docker/dockerfile:1
# check=skip=SecretsUsedInArgOrEnv
FROM jzaplet/documan:2.9.1-alpha

# Copy documentation source files into the image
COPY ../../docs /documan/data/docs
COPY ../../README.md /documan/data/README.md

# Set working directory
# Important for relative paths in DOCUMAN_DOCS_FILES or DOCUMAN_EXCLUDED_FILES
WORKDIR /documan/data

# Configure your project
ENV DOCUMAN_PROJECT_NAME='Filmshes'
ENV DOCUMAN_DOCS_FILES='**/*.md'
ENV DOCUMAN_EXCLUDED_FILES=''
ENV DOCUMAN_HTTP_PORT=3000
ENV DOCUMAN_OPENAI_EMBEDDING_MODEL='text-embedding-3-small'
ENV DOCUMAN_CHUNK_MAX_LEN=250

# Build-time args for PaaS platforms (e.g., EasyPanel) that pass secrets via --build-arg
# Locally, Docker secrets are used instead (mounted at /run/secrets/)
ARG DOCUMAN_LICENSE_KEY=''
ARG DOCUMAN_OPENAI_API_KEY=''

# Validate markdown files - checks for broken links, invalid frontmatter, etc.
# Fails the build if any issues are found
RUN /documan/bin/documan lint

# Import docs (license key optional - needed for >100 files)
# Uses Docker secret if available, otherwise falls back to ARG
RUN --mount=type=secret,id=DOCUMAN_LICENSE_KEY,required=false \
    if [ -f /run/secrets/DOCUMAN_LICENSE_KEY ]; then \
      export DOCUMAN_LICENSE_KEY=$(cat /run/secrets/DOCUMAN_LICENSE_KEY); \
    fi && \
    /documan/bin/documan import

# Vectorize docs (only runs if OpenAI API key is provided)
# Uses Docker secret if available, otherwise falls back to ARG
RUN --mount=type=secret,id=DOCUMAN_OPENAI_API_KEY,required=false \
    if [ -f /run/secrets/DOCUMAN_OPENAI_API_KEY ]; then \
      export DOCUMAN_OPENAI_API_KEY=$(cat /run/secrets/DOCUMAN_OPENAI_API_KEY); \
    fi && \
    if [ -n "${DOCUMAN_OPENAI_API_KEY}" ]; then /documan/bin/documan vectorize; fi

CMD ["/documan/bin/documan", "serve"]
