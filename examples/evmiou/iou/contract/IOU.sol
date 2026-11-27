// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
pragma solidity 0.8.26;

/// @title IOU
/// @notice On-chain mirror of the FSC Fabric IOU, ported to show FSC's multi-party protocol (the
///         borrower, the lender and an approver) enforced by an EVM contract instead of a Fabric
///         namespace. A create or update is only applied when the borrower, the lender and one of
///         the configured approvers have signed that exact change with an EIP-712 signature.
/// @dev    EIP-712 hashing here is a byte-for-byte mirror of the Go implementation in
///         `examples/evmiou/evm/eip712` and `examples/evmiou/iou/contract/contract.go`: domain
///         name "FSC IOU", version "1", the `Create`/`Update` typed structs below. ecrecover is used
///         with a low-s check and `v` restricted to {27,28}, as in Panurus's EndorsementVerifier.
contract IOU {
    struct Entry {
        uint256 amount;
        address borrower;
        address lender;
        uint256 version;
    }

    /// @dev secp256k1 group-order / 2. Signatures with `s` above this are malleable (EIP-2) and
    ///      rejected.
    uint256 private constant SECP256K1_N_HALF =
        0x7FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF5D576E7357A4501DDFE92F46681B20A0;
    uint256 private constant SIGNATURE_LENGTH = 65;
    uint256 private constant MIN_AMOUNT = 5;

    /// @dev keccak256("EIP712Domain(string name,string version,uint256 chainId,address
    ///      verifyingContract)")
    bytes32 private constant DOMAIN_TYPEHASH =
        keccak256("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)");
    bytes32 private constant DOMAIN_NAME_HASH = keccak256("FSC IOU");
    bytes32 private constant DOMAIN_VERSION_HASH = keccak256("1");

    /// @dev keccak256("Create(bytes32 id,uint256 amount,address borrower,address lender)")
    bytes32 private constant CREATE_TYPEHASH =
        keccak256("Create(bytes32 id,uint256 amount,address borrower,address lender)");
    /// @dev keccak256("Update(bytes32 id,uint256 amount,uint256 version)")
    bytes32 private constant UPDATE_TYPEHASH =
        keccak256("Update(bytes32 id,uint256 amount,uint256 version)");

    bytes32 private immutable _domainSeparator;

    mapping(address => bool) private _isApprover;
    mapping(bytes32 => Entry) private _ious;

    event Created(bytes32 indexed id, uint256 amount);
    event Updated(bytes32 indexed id, uint256 amount, uint256 version);

    error ZeroApprover();
    error DuplicateApprover(address approver);
    error IDAlreadyExists(bytes32 id);
    error IDDoesNotExist(bytes32 id);
    error AmountTooLow(uint256 amount, uint256 min);
    error AmountNotDecreasing(uint256 amount, uint256 current);
    error SameBorrowerLender(address party);
    error VersionMismatch(uint256 given, uint256 stored);
    error InvalidSignatureLength(uint256 length);
    error InvalidSignatureV(uint8 v);
    error HighSValue();
    error UnauthorizedBorrowerSignature(address signer);
    error UnauthorizedLenderSignature(address signer);
    error UnauthorizedApproverSignature(address signer);
    error ZeroSigner();
    error ApproverNotDistinct(address approver);

    /// @param approvers the authorized approver addresses (no zero, no duplicates). Fixed for the
    ///        lifetime of the contract: there is no on-chain governance to change the set.
    constructor(address[] memory approvers) {
        for (uint256 i = 0; i < approvers.length; i++) {
            address a = approvers[i];
            if (a == address(0)) revert ZeroApprover();
            if (_isApprover[a]) revert DuplicateApprover(a);
            _isApprover[a] = true;
        }
        _domainSeparator = keccak256(
            abi.encode(DOMAIN_TYPEHASH, DOMAIN_NAME_HASH, DOMAIN_VERSION_HASH, block.chainid, address(this))
        );
    }

    /// @notice Creates a new IOU at `id`, version 1. Reverts unless `id` is unused, `amount >= 5`,
    ///         `borrower != lender`, and the borrower, the lender and an approver have each signed
    ///         `Create{id, amount, borrower, lender}`.
    function create(
        bytes32 id,
        uint256 amount,
        address borrower,
        address lender,
        bytes calldata sigBorrower,
        bytes calldata sigLender,
        bytes calldata sigApprover
    ) external {
        if (_ious[id].version != 0) revert IDAlreadyExists(id);
        if (amount < MIN_AMOUNT) revert AmountTooLow(amount, MIN_AMOUNT);
        if (borrower == lender) revert SameBorrowerLender(borrower);

        bytes32 digest = _digest(keccak256(abi.encode(CREATE_TYPEHASH, id, amount, borrower, lender)));
        _checkSigners(digest, sigBorrower, borrower, sigLender, lender, sigApprover);

        _ious[id] = Entry({amount: amount, borrower: borrower, lender: lender, version: 1});
        emit Created(id, amount);
    }

    /// @notice Updates the IOU at `id` to `amount`, replacing `version`. Reverts unless `id` exists,
    ///         `amount` is strictly less than the current amount, `version` equals the stored
    ///         version, and the stored borrower, the stored lender and an approver have each signed
    ///         `Update{id, amount, version}`. The stored version is incremented on success.
    function update(
        bytes32 id,
        uint256 amount,
        uint256 version,
        bytes calldata sigBorrower,
        bytes calldata sigLender,
        bytes calldata sigApprover
    ) external {
        Entry storage e = _ious[id];
        if (e.version == 0) revert IDDoesNotExist(id);
        if (amount >= e.amount) revert AmountNotDecreasing(amount, e.amount);
        if (version != e.version) revert VersionMismatch(version, e.version);

        bytes32 digest = _digest(keccak256(abi.encode(UPDATE_TYPEHASH, id, amount, version)));
        _checkSigners(digest, sigBorrower, e.borrower, sigLender, e.lender, sigApprover);

        e.amount = amount;
        e.version = e.version + 1;
        emit Updated(id, amount, e.version);
    }

    /// @notice Reads the current state of the IOU at `id`. `version == 0` means `id` does not exist.
    function get(bytes32 id)
        external
        view
        returns (uint256 amount, address borrower, address lender, uint256 version)
    {
        Entry storage e = _ious[id];
        return (e.amount, e.borrower, e.lender, e.version);
    }

    /// @dev Checks that `sigBorrower`/`sigLender` recover to `borrower`/`lender` and that
    ///      `sigApprover` recovers to a configured approver distinct from both, all over the same
    ///      `digest`. The digest is identical for all three slots, so without the distinctness
    ///      check one key that is both a party and an approver could fill two slots with a single
    ///      signature.
    function _checkSigners(
        bytes32 digest,
        bytes calldata sigBorrower,
        address borrower,
        bytes calldata sigLender,
        address lender,
        bytes calldata sigApprover
    ) private view {
        address signerB = _recover(digest, sigBorrower);
        if (signerB != borrower) revert UnauthorizedBorrowerSignature(signerB);
        address signerL = _recover(digest, sigLender);
        if (signerL != lender) revert UnauthorizedLenderSignature(signerL);
        address signerA = _recover(digest, sigApprover);
        if (!_isApprover[signerA]) revert UnauthorizedApproverSignature(signerA);
        if (signerA == signerB || signerA == signerL) revert ApproverNotDistinct(signerA);
    }

    /// @dev The EIP-712 signing digest: keccak256(0x19 || 0x01 || domainSeparator || structHash).
    function _digest(bytes32 structHash) private view returns (bytes32) {
        return keccak256(abi.encodePacked(bytes1(0x19), bytes1(0x01), _domainSeparator, structHash));
    }

    /// @dev Recovers the signer of a canonical, non-malleable signature, reverting on any
    ///      malformation. ecrecover returns address(0) for a malformed signature (for example
    ///      r=0,s=0); rejecting it here, rather than leaving it to the callers' comparisons against
    ///      borrower/lender/approver, closes the case where a caller-supplied borrower or lender is
    ///      itself address(0).
    function _recover(bytes32 digest, bytes calldata sig) private pure returns (address) {
        if (sig.length != SIGNATURE_LENGTH) revert InvalidSignatureLength(sig.length);

        bytes32 r;
        bytes32 s;
        uint8 v;
        assembly {
            r := calldataload(sig.offset)
            s := calldataload(add(sig.offset, 32))
            v := byte(0, calldataload(add(sig.offset, 64)))
        }

        if (v != 27 && v != 28) revert InvalidSignatureV(v);
        if (uint256(s) > SECP256K1_N_HALF) revert HighSValue();

        address signer = ecrecover(digest, v, r, s);
        if (signer == address(0)) revert ZeroSigner();

        return signer;
    }
}
