#!/usr/bin/env python3

"""
fix-recursive-crd.py
Post-processes generated CRDs to fix recursive schema issues
"""

import sys
import yaml
import os
from pathlib import Path

def fix_recursive_crd(crd_file):
    """Fix recursive schema issues in the FolderTree CRD"""

    if not os.path.exists(crd_file):
        print(f"ERROR: CRD file not found: {crd_file}")
        return False

    print(f"🔧 Fixing recursive schema in {crd_file}...")

    # Read the CRD
    with open(crd_file, 'r') as f:
        crd = yaml.safe_load(f)

    # Navigate to the spec properties
    try:
        spec_props = crd['spec']['versions'][0]['schema']['openAPIV3Schema']['properties']['spec']['properties']

        # Fix the tree field - TreeNode can have recursive subfolders
        if 'tree' in spec_props:
            try:
                tree_props = spec_props['tree']['properties']
                # Fix the subfolders schema in TreeNode
                if 'subfolders' in tree_props:
                    tree_props['subfolders'] = {
                        'description': 'Subfolders is a list of child tree nodes',
                        'type': 'array',
                        'items': {
                            'type': 'object',
                            'x-kubernetes-preserve-unknown-fields': True
                        }
                    }
                    print("✅ Fixed tree.subfolders schema")
                else:
                    print("⚠️  subfolders field not found in tree schema")
            except KeyError as e:
                print(f"⚠️  Could not fix tree schema: {e}")
        else:
            print("⚠️  tree field not found in spec")

        # The folders array doesn't need fixing as it's not recursive
        if 'folders' in spec_props:
            print("✅ Folders schema is already correct (no recursion)")
        else:
            print("⚠️  folders field not found in spec")

    except KeyError as e:
        print(f"❌ Failed to navigate CRD structure: {e}")
        return False

    # Write the fixed CRD back
    try:
        with open(crd_file, 'w') as f:
            yaml.dump(crd, f, default_flow_style=False, sort_keys=False)
        print("✅ Successfully wrote fixed CRD")
        return True
    except Exception as e:
        print(f"❌ Failed to write CRD: {e}")
        return False

def main():
    crd_file = "config/crd/bases/rbac.kubevirt.io_foldertrees.yaml"

    if fix_recursive_crd(crd_file):
        print("🎉 CRD fix completed successfully!")
        sys.exit(0)
    else:
        print("❌ CRD fix failed!")
        sys.exit(1)

if __name__ == "__main__":
    main()
