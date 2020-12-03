BASE_DIR="$(cd $(dirname $0)/.. && pwd -P)"
OSSTP_LOAD=${OSSTP_LOAD:-~/workspace/osstpclients/bin/osstp-load.py}
OSM_API_KEY_FILE=${OSM_API_KEY_FILE:-~/.osmapikey}
REMAPPER="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"/osstp.yaml

pushd $BASE_DIR
    osstptool generate --vendor-path=vendor azure-nozzle github.com/vmware-tanzu/nozzle-for-microsoft-azure-log-analytics --remapper-definitions-file=$BASE_DIR/hack/osstp.yaml
    osstptool download
    python2 $OSSTP_LOAD -R azure-nozzle/2.0.0 osstp_golang.yml -I "Distributed - Static Link w/ TP" -A $OSM_API_KEY_FILE
popd
