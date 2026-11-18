# Tethering validation

NetPilot runs on the desktop. The adapter list uses OS interface names; a name match is a candidate, not proof that the link is tethered. For a Wi-Fi hotspot or a USB link with a generic name, identify the adapter in **Systems** and select it manually under **Mobile**. Tether profiles configure only the NetPilot tunnel's MTU and IPv6 policy; desktop TTL and Hop Limit are separate, explicit settings.

## Measured in this change

One NetPilot run on a Linux ARM64 VM's `eth0` (5-second limit per phase, four streams, 10 MiB cap per direction):

| Download | Upload | Idle latency | Loaded latency | Bufferbloat |
| ---: | ---: | ---: | ---: | ---: |
| 71.2 Mbps | 127.1 Mbps | 97 ms | 182 ms | +84 ms (C) |

The other adapter returned no latency samples. NetPilot now leaves the grade blank in that case instead of reporting an A. The desktop reported TTL 64 and Hop Limit 64; the demo's dry run did not change either value. No iOS or Android tether was confirmed on the test host, so there are no handset or carrier measurements for this change.

## Testing a tethered link

1. Connect to an iPhone Personal Hotspot or Android tether over Wi-Fi or USB. Confirm the desktop is actually routing over that connection; do not assume the adapter name identifies the phone.
2. In **Mobile → Tethering & Links**, run the speedtest with **Merged pool** off and record the tether adapter's result. Use the same duration and location for each comparison. Tests transfer real data and may count against your plan.
3. Record the phone OS, desktop OS, connection type, and the median download, upload, idle latency, and loaded latency from repeated runs. Report missing samples as unavailable.

These measurements do not reveal carrier traffic classification. If an older NetPilot version changed desktop packet headers through a tether profile, **Mobile → TTL & Hop Limit → Restore Originals** can restore the captured values; reverting a tunnel profile no longer touches desktop packet headers.
