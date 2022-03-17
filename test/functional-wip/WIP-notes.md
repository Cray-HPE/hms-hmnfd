Things Ive learned.

This docker compose is pretty much ready to go.

It will setup everyting it needs, you do have to post into the fake subscribers at /do_subscribe to get them to kick off the subscription chain
then you have to have something either trigger the state change in HSM or just directly LIE to hmnfd about a SCN.  

Right now the HMNFD to HSM 'I want to subscribe please' path is getting a 400... not sure why.  The tavern tests are manipulating the environment to work
