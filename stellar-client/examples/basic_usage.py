#!/usr/bin/env python3
"""
Basic Stellar Client Usage Example

This example demonstrates the fundamental operations with Stellar client
using the new docker-py-like API:
- Connecting to a Stellar node
- Listing discovered devices
- Pinging devices
- Managing security policies
"""

import stellar_client
from stellar_client import StellarException

def main():
    print("Stellar Client - Basic Usage Example")
    print("=" * 40)
    
    try:
        # Create client (like docker.from_env())
        with stellar_client.from_env() as client:
            print("✓ Connected to Stellar node")
            
            # Get node information
            node_info = client.info()
            print(f"Node ID: {node_info.id}")
            print(f"Addresses: {node_info.addresses}")
            print()
            
            # List discovered devices (like client.containers.list())
            print("Discovering devices...")
            devices = client.devices.list()
            print(f"Found {len(devices)} devices:")
            
            for i, device in enumerate(devices, 1):
                print(f"  {i}. {device.id}")
                device_model = device.model
                if device_model.sys_info:
                    print(f"     Platform: {device_model.sys_info.platform}, CPU: {device_model.sys_info.cpu}")
            print()
            
            if not devices:
                print("No devices found. Start additional Stellar nodes to see them here.")
                return
            
            # Ping devices
            print("Pinging devices...")
            for device in devices:
                try:
                    device.ping()
                    print(f"  ✓ {device.id}: Online")
                except Exception as e:
                    print(f"  ✗ {device.id}: Failed - {e}")
            print()
            
            # Get detailed device information
            if devices:
                print("Getting detailed device info...")
                try:
                    device = devices[0]
                    device_info = device.info()
                    
                    print(f"Device:")
                    print(f"  ID: {device.id}")
                    if device_info.get("ReferenceToken"):
                        print(f"  Reference Token: {device_info['ReferenceToken']}")
                    if device_info.get("SysInfo"):
                        sys_info = device_info["SysInfo"]
                        print(f"  System Info:")
                        print(f"    - Platform: {sys_info.get('Platform')}")
                        print(f"    - CPU: {sys_info.get('CPU')}")
                        print(f"    - GPU: {sys_info.get('GPU')}")
                        if sys_info.get("RAM"):
                            print(f"    - Memory: {sys_info['RAM'] // 1024} GB")
                except Exception as e:
                    print(f"  Failed to get device info: {e}")
                print()
            
            # Security policy management
            print("Security Policy Management:")
            try:
                policy = client.policy.get()
                print(f"Policy enabled: {policy.enable}")
                print(f"Whitelisted devices: {len(policy.whitelist)}")
                
                if devices and policy.enable:
                    # Add a device to whitelist
                    device_id = devices[0].id
                    if device_id not in policy.whitelist:
                        print(f"Adding {device_id} to whitelist...")
                        client.policy.add_to_whitelist(device_id)
                        print(f"  ✓ Whitelist updated")
                        
                        # Verify whitelist
                        updated_whitelist = client.policy.get_whitelist()
                        print(f"Current whitelist: {len(updated_whitelist)} devices")
                        
            except Exception as e:
                print(f"Policy management error: {e}")
            print()
            
        print("✓ Disconnected from Stellar node")
        
    except StellarException as e:
        print(f"Stellar Error: {e}")
        return 1
    except Exception as e:
        print(f"Unexpected error: {e}")
        return 1
        
    return 0


if __name__ == "__main__":
    exit(main())
