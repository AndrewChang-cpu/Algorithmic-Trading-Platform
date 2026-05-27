# Stage 1: Build the C# plugin
FROM mcr.microsoft.com/dotnet/sdk:8.0 AS build
WORKDIR /src
COPY KafkaDataQueueHandler.csproj .
RUN dotnet restore
COPY KafkaDataQueueHandler.cs .
RUN dotnet publish -c Release -o /app/publish

# Stage 2: Final LEAN image with plugin and auxiliary data files
FROM quantconnect/lean@sha256:9cf7d11aee595361869e0216f515ae6b3f58ad41f422bfc3d70a821004ad5990 AS final

# Install the compiled plugin
# LEAN loads plugins from its Launcher directory
COPY --from=build /app/publish/*.dll /Lean/Launcher/bin/Debug/
COPY --from=build /app/publish/*.deps.json /Lean/Launcher/bin/Debug/

# Copy required auxiliary data files that LEAN needs to run without errors.
# These are already present in the quantconnect/lean image under /Lean/Data/.
# We copy them to the data-folder that our config.json will point to.
# The data-folder will be /lean/data (mounted from host via docker run -v).
# Since the host volume is empty initially, we bake these into a known path
# that Celery can reference when building the docker run command.
# Strategy: copy to /lean-aux-data/ so Celery can bind-mount or copy them.
RUN mkdir -p /lean-aux-data/symbol-properties \
    /lean-aux-data/equity/usa/map_files \
    /lean-aux-data/equity/usa/factor_files

# Copy from LEAN's bundled sample data
RUN if [ -f /Lean/Data/symbol-properties/symbol-properties-database.csv ]; then \
      cp /Lean/Data/symbol-properties/symbol-properties-database.csv /lean-aux-data/symbol-properties/; \
    fi && \
    if [ -d /Lean/Data/equity/usa/map_files ]; then \
      cp -r /Lean/Data/equity/usa/map_files/. /lean-aux-data/equity/usa/map_files/; \
    fi && \
    if [ -d /Lean/Data/equity/usa/factor_files ]; then \
      cp -r /Lean/Data/equity/usa/factor_files/. /lean-aux-data/equity/usa/factor_files/; \
    fi

WORKDIR /Lean/Launcher
