#!/bin/bash

tar -xzf cyc-cde.tar.gz

echo '#!/bin/bash' > cyclus
echo "$PWD/cyc-cde/cde-exec /home/robert/cyc/install/bin/cyclus \$@" >> cyclus
chmod a+x cyclus

echo '#!/bin/bash' > cycobj
echo "$PWD/cyc-cde/cde-exec $(which cycobj) \$@" >> cycobj
chmod a+x cycobj

