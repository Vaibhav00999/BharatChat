"""Offline policy invariants and optional concrete documents for AWS validation."""
import argparse
import json
from pathlib import Path
import re

from cfnlint.decode import decode


ROOT = Path(__file__).resolve().parents[1]


def load(name):
    template, errors = decode(str(ROOT / name))
    assert not errors, errors
    return template


def resolve(value, refs, attrs):
    if isinstance(value, list):
        return [resolve(item, refs, attrs) for item in value]
    if not isinstance(value, dict):
        return value
    if set(value) == {"Ref"}:
        return refs[value["Ref"]]
    if set(value) == {"Fn::GetAtt"}:
        key = value["Fn::GetAtt"]
        return attrs[".".join(key) if isinstance(key, list) else key]
    if set(value) == {"Fn::Sub"}:
        assert isinstance(value["Fn::Sub"], str)
        return re.sub(r"\$\{([^}]+)\}", lambda m: refs[m[1]], value["Fn::Sub"])
    return {key: resolve(item, refs, attrs) for key, item in value.items()}


def check(account, vpc, subnet, ami, output=None):
    assert re.fullmatch(r"\d{12}", account)
    assert re.fullmatch(r"vpc-[a-f0-9]+", vpc)
    assert re.fullmatch(r"subnet-[a-f0-9]+", subnet)
    assert re.fullmatch(r"ami-[a-f0-9]+", ami)
    template = load("provisioning-iam.yml")
    host = load("ec2-host.yml")
    refs = {"AWS::Partition": "aws", "AWS::AccountId": account,
            "VpcId": vpc, "PublicSubnetId": subnet, "UbuntuImage": ami}
    attrs = {"HostRole.Arn": f"arn:aws:iam::{account}:role/bharatchat/runtime/bharatchat-prelaunch-host",
             "HostProfile.Arn": f"arn:aws:iam::{account}:instance-profile/bharatchat/runtime/bharatchat-prelaunch-host"}
    provisioning = template["Resources"]["ProvisioningPolicy"]
    assert provisioning["Type"] == "AWS::IAM::ManagedPolicy", "Provisioning must not exhaust the user's inline quota"
    assert provisioning["Properties"]["Users"] == ["bharatchat-deployer"]
    assert provisioning["Properties"]["ManagedPolicyName"] == "BharatChatPrelaunchProvisioning"
    assert provisioning["Properties"]["Path"] == "/bharatchat/provisioning/"
    policy = resolve(provisioning["Properties"]["PolicyDocument"], refs, attrs)
    statements = {s["Sid"]: s for s in policy["Statement"]}
    iam_actions = set()
    for statement in policy["Statement"]:
        actions = statement["Action"]
        actions = [actions] if isinstance(actions, str) else actions
        assert statement["Effect"] == "Allow"
        assert all("*" not in a for a in actions), "No wildcard actions"
        iam_actions.update(a for a in actions if a.startswith("iam:"))
        for action in actions:
            if action.startswith("ec2:"):
                assert statement["Condition"]["ForAnyValue:StringEquals"]["aws:CalledVia"] == ["cloudformation.amazonaws.com"]
                resources = statement["Resource"]
                resources = [resources] if isinstance(resources, str) else resources
                assert all(r.startswith("arn:aws:ec2:ap-south-1:") for r in resources)
            assert action not in ("cloudformation:UpdateStack", "cloudformation:DeleteStack", "secretsmanager:GetSecretValue")
    assert iam_actions == {"iam:GetInstanceProfile", "iam:PassRole"}
    assert statements["PassOnlyHostRoleToEC2"]["Resource"] == attrs["HostRole.Arn"]
    assert statements["PassOnlyHostRoleToEC2"]["Condition"]["StringEquals"]["iam:PassedToService"] == "ec2.amazonaws.com"
    create = statements["CreateOnlyPilotStackWithoutServiceRole"]
    assert create["Resource"] == f"arn:aws:cloudformation:ap-south-1:{account}:stack/bharatchat-prelaunch/*"
    assert create["Condition"]["Null"]["cloudformation:RoleArn"] == "true"
    launch = statements["LaunchOnlyTaggedSmallHostWithIMDSv2"]["Condition"]
    assert launch["StringEquals"]["ec2:MetadataHttpTokens"] == "required"
    assert launch["StringEquals"]["aws:RequestTag/Project"] == "BharatChat"
    assert launch["ArnEquals"]["ec2:InstanceProfile"] == attrs["HostProfile.Arn"]
    assert statements["LaunchEncryptedBoundedVolumes"]["Condition"]["Bool"]["ec2:Encrypted"] == "true"
    assert statements["LaunchPinnedImageAndSubnet"]["Resource"][:2] == [
        f"arn:aws:ec2:ap-south-1::image/{ami}", f"arn:aws:ec2:ap-south-1:{account}:subnet/{subnet}"]
    for resource in host["Resources"].values():
        assert not resource["Type"].startswith("AWS::IAM::")
        assert {"Key": "Project", "Value": "BharatChat"} in resource["Properties"]["Tags"]
    assert host["Resources"]["Host"]["Properties"]["IamInstanceProfile"] == {"Ref": "HostInstanceProfile"}
    assert host["Parameters"]["UbuntuImage"]["Type"] == "AWS::EC2::Image::Id"
    # IAM user inline policies share 2,048 characters; managed policies have 6,144 each.
    onboard = json.loads((ROOT / "deployer-onboarding-policy.json").read_text())
    inline_size = len(json.dumps(onboard, separators=(",", ":")))
    managed_size = len(json.dumps(policy, separators=(",", ":")))
    assert inline_size <= 2048, inline_size
    assert managed_size <= 6144, managed_size
    assert inline_size + managed_size > 2048, "Fixture must exercise the inline-quota regression"
    if output:
        destination = Path(output).resolve()
        assert destination == ROOT / "runtime", "Render only into the ignored runtime directory"
        destination.mkdir(exist_ok=True)
        (destination / "provisioning-policy.json").write_text(json.dumps(policy, indent=2) + "\n")
    print(f"PASS: policy and host invariants; user inline {inline_size}/2048; managed provisioning {managed_size}/6144.")


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--account", default="123456789012")
    parser.add_argument("--vpc", default="vpc-1234abcd")
    parser.add_argument("--subnet", default="subnet-1234abcd")
    parser.add_argument("--ami", default="ami-1234abcd")
    parser.add_argument("--output")
    args = parser.parse_args()
    check(args.account, args.vpc, args.subnet, args.ami, args.output)
