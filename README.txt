This is the HMS/HSM notification fanout service -- hmnfd.  It fans out State 
Change Notifications (SCNs) from the State Manager.

hmnfd will do the following:

 o Subscribe for SCNs from the State Manager.
 o Receive subscription requests from nodes for SCNs.
 o Forwards SCNs received from the State Manager to subscribers.
 o The forwarded SCNs only contain nodes the subscriber is
   interested in.
 o Prune subscriptions when nodes fail to receive SCNs consistently,
   or when an SCN is received from State manager indicating subscribed
   nodes are unavailable.

This is the initial implementation.  All of the above features are working.

Currently scale testing shows that an SCN containing 1000 nodes can be 
forwarded to 2000 subscribers in about .75 seconds.

FUTURE WORK:

 o Insure hmnfd scales to at least 50k nodes.  Horizontal scaling can help,
   but may require a different fanout scheme.  Currently, whichever hmnfd
   instance receives the SCN (via RR load balancing), it will forward 
   the SCN to all subscribers.   This may need to change if the current
   implementation takes too long in large systems.

 o Segmented fanout.  This would change the current fanout scheme to one 
   where each hmnfd instance would handle a subset of SCN fanout, and
   each instance would participate in every SCN fanout operation.

 o Make sure SCNs are aggregated.  If there are frequent small SCNs received 
   from the State Manager, hmnfd should aggregate these into fewer larger
   SCNs.  The State manager may do this, or hmnfd may do it, wherever it 
   makes the most sense.

 o Create a more combined "Unavailable" state, which is any of STANDBY,
   HALT, ON, OFF, DISABLED, EMPTY.  Subscribers could use a single
   subscription and get notifications for any of these.

 o Use "SCN masking".  In the state sequence READY->STANDBY->HALT->OFF,
   any state past STANDBY should result in not sending any of the subsequent
   SCNs.  This may have to be combined with the use of the "Unavailable"
   state mentioned above.

 o Accomodate http and https to subscribers.  Currently only http is supported.

 o Batch up subscription DELETE operations, as they are somewhat expensive.
   Shouldn't be needed until we scale up to large systems.

 o Conform to whatever RBAC scheme the HSM uses.  Currently uses simple cert/key
   authentication.


