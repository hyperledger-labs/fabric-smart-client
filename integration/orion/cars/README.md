# Car Registry

We took [Orion's car registry demo](https://github.com/hyperledger-labs/orion-sdk-go/tree/main/examples/cars) 
and reimplemented it using the Fabric Smart Client

## Setting

An imaginary and oversimplified car registry, in which the DMV (department of motor vehicles) keeps track of all cars and their ownership.

## Roles
* A car registry owned by the DMV, with user `dmv`.
    * The `dmv` approves a mint request of a new car by a car dealer, inserting a new car record into the database.
    * The `dmv` approves a transfer of ownership between a car owner (seller) to a new car owner (buyer).
* Car dealer, with user `dealer`.
    * Issues a mint request for a new car, which the `dmv` must approve.
    * The `dealer` can then transfer ownership by selling the car to a new owner, say `alice`, the buyer.
* Owners `alice`, `bob` (and `dealer`)
    * Can own a car.
    * Can transfer ownership of the car they own.
    * Can approve the reception (buying) of a car, and assume ownership of it. 
 