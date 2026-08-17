# Binary is pre-built by the CI workflow (native arch) before docker buildx.
# No build stage needed — just copy into a minimal runtime image.
FROM gcr.io/distroless/static-debian12
COPY menu /menu
EXPOSE 8080
ENTRYPOINT ["/menu"]
CMD ["serve"]
