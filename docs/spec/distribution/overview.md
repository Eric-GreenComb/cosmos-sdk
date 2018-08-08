# Distribution

## Overview

This _simple_ distribution mechanism describes a functional way to passively 
distribute rewards between validator and delegators. Note that this mechanism does 
not distribute funds in as precisely as active reward distribution and will therefor
be upgraded in the future. 

The mechanism operates as follows. Collected rewards are pooled globally and
divided out passively to validators and delegators. Each validator has the
opportunity to charge commission to the delegators on the rewards collected on
behalf of the delegators by the validators.  Fees are paid directly into a
global reward pool, and validator proposer-reward pool. Due to the nature of
passive accounting whenever changes to parameters which affect the rate of reward
distribution occurs, withdrawal of rewards must also occur when: 
 
 - withdrawing one must withdrawal the maximum amount they are entitled
   too, leaving nothing in the pool, 
 - bonding, unbonding, or re-delegating tokens to an existing account a
   full withdrawal of the rewards must occur (as the rules for lazy accounting
   change), 
 - a validator chooses to change the commission on rewards, all accumulated 
   commission rewards must be simultaneously withdrawn.

The above scenarios are covered in `triggers.md`.

The distribution mechanism outlines herein is used to lazily distribute the
following rewards between validators and associated delegators:
 - multi-token fees to be socially distributed, 
 - proposer reward pool, 
 - inflated atom provisions, and
 - validator commission on all rewards earned by their delegators stake

Fees are pooled within a global pool, as well as validator specific
proposer-reward pools.  The mechanisms used allow for validators and delegators
to independently and lazily  withdrawn their rewards.  

Within this spec 

As a part of the lazy computations, each validator and delegator holds an
accumulation term which is used to estimate what their approximate fair portion
of tokens held in the global pool is owed to them. This approximation of owed
rewards would be equivalent to the active distribution under the situation that
there was a constant flow of incoming reward tokens every block. Because this
is not the case, the approximation of owed rewards will deviate from the active
distribution based on fluctuations of incoming reward tokens as well as timing
of reward withdrawal by other delegators and validators from the reward pool.


## Affect on Staking

Charging commission on Atom provisions while also allowing for Atom-provisions
to be auto-bonded (distributed directly to the validators bonded stake) is
problematic within DPoS.  Fundamentally these two mechnisms are mutually
exclusive. If there are atoms commissions and auto-bonding Atoms, the portion
of Atoms the reward distribution calculation would become very large as the Atom
portion for each delegator would change each block making a withdrawal of rewards
for a delegator require a calculation for every single block since the last
withdrawal. In conclusion we can only have atom commission and unbonded atoms
provisions, or bonded atom provisions with no Atom commission, and we elect to
implement the former. Stakeholders wishing to rebond their provisions may elect
to set up a script to periodically withdraw and rebond rewards. 
