FROM ubuntu:xenial

RUN apt update && apt install zip curl git jq python -y
ADD https://github.com/cf-platform-eng/tile-generator/releases/download/v14.0.5/tile_linux-64bit tile
RUN chmod +x tile && mv tile /usr/local/bin
ADD https://github.com/cloudfoundry/bosh-cli/releases/download/v6.3.0/bosh-cli-6.3.0-linux-amd64 bosh
RUN chmod +x bosh && mv bosh /usr/local/bin
ADD https://github.com/pivotal-cf/om/releases/download/5.0.0/om-linux-5.0.0 om
RUN chmod +x om && mv om /usr/local/bin


RUN apt-get update && apt-get install ca-certificates curl apt-transport-https lsb-release gnupg -y
RUN curl -sL https://packages.microsoft.com/keys/microsoft.asc | gpg --dearmor | tee /etc/apt/trusted.gpg.d/microsoft.gpg > /dev/null
RUN echo "deb [arch=amd64] https://packages.microsoft.com/repos/azure-cli/ xenial main" | tee /etc/apt/sources.list.d/azure-cli.list
RUN apt-get update && apt-get install azure-cli -y
RUN az extension add --name log-analytics

