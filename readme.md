cran proxy
----------

enables precise configuration management for production R installations.

to run (assuming you already have http://golang.org installed):

    git clone https://github.com/xoba/cran.git && cd cran && ./run.sh

for some help:

    ./run.sh -help

to contribute to the project, feel free to issue a pull request!

    ./ide.sh # invoke the editor/ide :-)

as an example, here's how i often use it; first, i start the server:

    ./run.sh -port 8080

then, i pipe an installation script into R:

    curl http://localhost:8080/install.r?packages=glmnet,Cairo,plyr,ggplot2 | R --vanilla

that script looks something like this:

    is.installed <- function(mypkg) is.element(mypkg, installed.packages()[,1])
    mirror = "http://127.0.0.1:8080"
    attempts = 10
    pause = 3

    #
    # package "glmnet"
    #
    if (!is.installed("glmnet")) {
    for (out in 1:attempts) {
    install.packages("glmnet",repo=mirror)
    if (is.installed("glmnet")) { break; } else { Sys.sleep(pause); }
    }
    if (!is.installed("glmnet")) { quit(save='no',status=1) }
    }

    #
    # package "Cairo"
    #
    if (!is.installed("Cairo")) {
    for (out in 1:attempts) {
    install.packages("Cairo",repo=mirror)
    if (is.installed("Cairo")) { break; } else { Sys.sleep(pause); }
    }
    if (!is.installed("Cairo")) { quit(save='no',status=1) }
    }

    #
    # package "plyr"
    #
    if (!is.installed("plyr")) {
    for (out in 1:attempts) {
    install.packages("plyr",repo=mirror)
    if (is.installed("plyr")) { break; } else { Sys.sleep(pause); }
    }
    if (!is.installed("plyr")) { quit(save='no',status=1) }
    }

    #
    # package "ggplot2"
    #
    if (!is.installed("ggplot2")) {
    for (out in 1:attempts) {
    install.packages("ggplot2",repo=mirror)
    if (is.installed("ggplot2")) { break; } else { Sys.sleep(pause); }
    }
    if (!is.installed("ggplot2")) { quit(save='no',status=1) }
    }
