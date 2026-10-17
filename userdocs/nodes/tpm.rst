.. _tpm:

=============================
TPM (Trusted Platform Module)
=============================

Warewulf can use the TPM chip in each node to make sure a node's runtime
overlay is delivered only to the right machine, and only when that machine
booted exactly what the Warewulf server sent it.

Why use it
==========

The runtime overlay often carries a node's most sensitive files: SSH host keys,
munge keys, credentials for site services. Without TPM support, any machine on
the provisioning network that presents a node's hardware address (and asset
key, if one is set) can download that node's runtime overlay.

With TPM support enabled for a node, the Warewulf server sends the runtime
overlay only after the node proves, with its TPM:

* that it is the same physical machine that was enrolled as that node, so a
  rogue or spoofed machine cannot impersonate it;
* that it booted the kernel and images the server sent, through the expected
  firmware and boot loader, so a tampered boot path is detected;
* that the proof is fresh, so a recording of an earlier boot cannot be
  replayed.

How it works
============

You do not need to know how TPMs work to use this feature, but the terms below
appear in ``wwctl`` output and in logs.

TPM
  A security chip on the motherboard (or built into the firmware). It holds
  keys that never leave it and records what the machine booted.

Measurement and PCRs
  As a machine boots, each stage (firmware, boot loader) records a fingerprint
  (a SHA-256 hash) of what it loads next into the TPM's *Platform
  Configuration Registers* (PCRs). A PCR can only be extended, never set or
  rewound, so its final value summarizes everything recorded into it. GRUB
  records each file it loads (the kernel, the images, ``grub.cfg``) into PCR 9.

Event log
  The list of everything that was measured, in order. Replaying it must
  reproduce the PCR values, which lets the server see each individual
  measurement.

Endorsement key (EK)
  A key built into the TPM at manufacture. It identifies the TPM, and so the
  machine.

Quote
  A statement of the current PCR values, signed inside the TPM. The server
  includes a random, single-use *nonce* in each request, and the quote must
  cover it, so every quote is fresh.

Enrollment
  The first time a node attests, the server records its EK and the PCRs that
  describe its firmware and boot loader (its *baseline*: PCRs 0, 2, 4 and 7).
  Later attestations must match. Accepting the first TPM a node presents is
  called *trust on first use*; see `Enrollment and its limits`_. Enrollment
  completes only when the node uses the secret from that first attestation,
  which proves the quote came from the TPM holding that EK.

Secret
  When a node's attestation passes, the server sends it a secret encrypted so
  that only that node's TPM can decrypt it. The node presents the secret to
  download its runtime overlay.

.. graphviz::

  digraph G {
      node [shape=box];
      rankdir=LR;
      boot [label="node boots via GRUB;\nfiles measured into PCR 9"];
      req [label="wwclient requests\nruntime overlay"];
      nonce [label="server: 401, attest;\nnode gets a nonce"];
      quote [label="node sends quote\nand event log"];
      verify [label="server verifies: fresh, enrolled TPM,\nbaseline, boot files it sent"];
      secret [label="server sends secret\nonly that TPM can decrypt"];
      overlay [label="node fetches runtime\noverlay with the secret"];
      boot -> req -> nonce -> quote -> verify -> secret -> overlay;
  }

All of this is automatic. ``wwclient`` attests when the server asks it to,
saves the secret in ``/warewulf/secret`` (readable only by root), and attests
again whenever the server rejects the secret.

Requirements
============

* TLS enabled on the Warewulf server (``warewulf:tls``). Without TLS, the
  server refuses the runtime overlay of every node that has TPM enabled.
* GRUB boot (``warewulf:grubboot``). GRUB is the boot loader that measures the
  files it loads. iPXE does not, so nodes booted by iPXE cannot attest.
* A TPM 2.0 enabled in each node's firmware.
* GRUB with its ``tpm`` module, as in distribution-signed GRUB builds. Any GRUB
  modules must be built into the GRUB image: modules loaded at boot are
  measured too, but they are not files Warewulf sent, so attestation fails.

``wwctl check`` reports whether the server meets these requirements for the
nodes that have TPM attestation enabled, and lists TPM changes awaiting
approval.

Quickstart
==========

Enable TLS and GRUB boot in ``warewulf.conf``:

.. code-block:: yaml

   warewulf:
     tls: true
     grubboot: true

Then enable TPM attestation on a profile or on individual nodes:

.. code-block:: shell

   wwctl profile set default --tpm=true
   # or
   wwctl node set n1 --tpm=true

Boot the nodes. Each enrolls on its first attestation. Check the result:

.. code-block:: console

   # wwctl node tpm list
   NODE  STATE     MANUFACTURER  EK (SHA256)          LAST ATTEST              SECRET
   n1    ENROLLED  Infineon      3f2a9c0e4b1d7a65...  ok 2026-10-17 09:12:44   current

``wwctl node status`` shows ``TPM`` in its SECURITY column for nodes whose
runtime overlay was released after attestation.

What is and is not protected
============================

* Only the runtime overlay is withheld until attestation. The kernel, image and
  system overlay are served as before, and are protected only by the asset key
  (if set). Keep secrets in the runtime overlay.
* The asset key is still checked for every request. TPM attestation is in
  addition to it, not instead of it.
* With dracut two-stage boot, GRUB measures the kernel and initramfs, but the
  image and system overlay that dracut downloads are not measured.
* The kernel command line and GRUB commands (measured into PCR 8) are not yet
  checked. Someone with console access could edit a GRUB menu entry without
  failing attestation; set a GRUB password to prevent menu edits.

Enrollment and its limits
-------------------------

A node is enrolled with the first TPM that attests for it and proves it holds
that TPM. If a different machine with its own TPM attests first, using the
node's hardware address and asset key, it is enrolled instead, and the real
node is then refused. Check the EK fingerprints
in ``wwctl node tpm list`` after enabling TPM on new nodes. To compare a
fingerprint on the node itself, hash the TPM's endorsement public key, for
example with ``tpm2_readpublic``.

Day-to-day operations
=====================

Changing images and overlays
  Nothing to do. The server records each file it sends, so a node that boots a
  new kernel or image attests normally.

Firmware updates and BIOS changes
  A change to the firmware, option ROMs, boot loader or Secure Boot settings
  changes the baseline. The node's next attestation is refused. ``wwclient``
  then proves the change comes from the node's own TPM, by decrypting a
  credential only that TPM can decrypt, and the server records the change as a
  *candidate*:

  .. code-block:: console

     # wwctl node tpm verify n1
     n1: CANDIDATE
       enrolled TPM:  Infineon, EK sha256 3f2a... (since 2026-09-01 10:02:11)
       candidate:     9c41d07be2a35f18: baseline_changed, PCRs [0] differ; Infineon, EK sha256 3f2a... (seen 2026-10-17 09:12:44)
                      if expected, accept it with: wwctl node tpm approve n1 --candidate 9c41d07be2a35f18

  If the change is expected, approve it by its candidate ID. The node attests
  successfully on its next attempt (``wwclient`` retries on its own):

  .. code-block:: shell

     wwctl node tpm approve n1 --candidate 9c41d07be2a35f18

  The ID makes sure you approve the change you inspected. Each confirmed change
  replaces the node's candidate, so if the node has reported a different change
  since you ran ``verify``, the ID no longer matches and approval fails; check
  the node again.

  ``--all`` approves firmware and boot loader changes (``baseline_changed``) on
  every node, or on the nodes you name, for example after updating the firmware
  across a rack. Because a change is recorded only after the node proves it
  came from its enrolled TPM, a ``baseline_changed`` candidate shows that TPM's
  real measurements. ``--all`` still doesn't pin each candidate, so use it right
  after a change you made.

Replacing a node's hardware
  A new motherboard or TPM has a different EK, which is also recorded as a
  candidate (``ek_changed``). The proof shows only that the change came from
  *a* TPM, not that it is this node's, so compare the candidate's EK
  fingerprint with the new hardware before approving it by its candidate ID as
  above. ``--all`` never approves a different TPM.

Forcing a node to attest again
  ``wwctl node tpm reset n1`` discards the node's secret; it attests again at
  its next runtime overlay request. ``--enrollment`` also forgets the enrolled
  TPM, so the next attestation enrolls whatever TPM the node presents.

Commands
========

* ``wwctl node tpm list [NODES]``: the TPM state of nodes. ``--json`` for
  scripts.
* ``wwctl node tpm verify [NODES]``: details for each node, re-verifying the
  evidence behind its current secret. Evidence for a secret the node hasn't
  used yet is marked "unconfirmed". ``--eventlog`` (and ``--pcr``) shows the
  measurements. ``--file FILE`` does the same for a copy of a node's state
  file, for example from a backup or another server.
* ``wwctl node tpm approve NODE --candidate ID``: accept a node's candidate.
  ``wwctl node tpm approve --all [NODES]``: accept baseline changes in bulk.
* ``wwctl node tpm reset [--enrollment] NODES``: make nodes attest again.

The server keeps each node's TPM state in
``LOCALSTATEDIR/warewulf/tpm/NODE.json``, readable only by root. It holds no
secrets: only hashes of issued secrets. ``wwctl clean`` removes the state of
deleted nodes.

Troubleshooting
===============

When attestation fails, ``wwctl node tpm list`` and ``wwctl node tpm verify``
show the reason, the node's ``wwclient`` logs it, and ``warewulfd`` logs it
with more detail. ``wwclient`` retries after 30 seconds, then less often.

A successful attestation, or a changed TPM or baseline, is shown as the last
attempt only once the node has proven it holds its TPM. Attempts that fail
verification can't prove where they came from, so they are shown as received:
treat those as a diagnostic.

.. list-table::
   :header-rows: 1

   * - Reason
     - Meaning
   * - ``baseline_changed``
     - The firmware, option ROMs, boot loader or Secure Boot settings differ
       from enrollment. Approve the candidate if expected.
   * - ``ek_changed``
     - A different TPM attested for this node. Approve if the hardware was
       replaced; otherwise investigate.
   * - ``boot_files_unknown``
     - The node booted a file this server did not send it: a different
       kernel, image or GRUB configuration, or a GRUB module loaded at boot.
   * - ``no_boot_files``
     - Nothing was measured into PCR 9: the node did not boot through GRUB
       (for example, it booted with iPXE or from local disk).
   * - ``no_kernel_measurement``
     - GRUB measured files, but no kernel the server sent.
   * - ``eventlog_replay``
     - The event log does not reproduce the quoted PCR values.
   * - ``quote_invalid``
     - The quote is not a valid TPM signature over the server's nonce.
   * - ``ek_cert_mismatch``
     - The TPM's EK certificate is for a different key.
   * - ``bad_evidence``
     - The node sent malformed or incomplete data.

To run an attestation by hand on a node, for example while debugging, stop
the ``wwclient`` service first: with ``warewulf:secure``, both use the same
privileged source port, so a second ``wwclient`` can't connect while the
service runs.

.. code-block:: shell

   systemctl stop wwclient
   /warewulf/wwclient --attest
   systemctl start wwclient

It prints whether attestation succeeded and saves the secret, but never prints
the secret.
