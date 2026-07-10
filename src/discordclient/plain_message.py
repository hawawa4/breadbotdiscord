from pathlib import Path
from typing import Tuple

import discord
from loguru import logger

from inference.predict import ImageData, InferenceClient, PredictResponse
from settings import SETTINGS


class FreeMessageHandler:
    @staticmethod
    def map_confidence_to_sentiment(confidence: float, label: str) -> str:
        """Translate a confidence percentage to text indicating how accurate the element is

        Args:
            confidence (float): Confidence value
            label (str): Label for the confidence

        Returns:
            str: Value for the confidence specified
        """
        label = label.replace("_", " ")
        if confidence < 0.3:
            return f"{label}, but I wouldn't trust it,"
        elif confidence < 0.4:
            return f"not sure about {label}"
        elif confidence < 0.5:
            return f"{label} is unlikely"
        elif confidence < 0.6:
            return f"slightly possible {label}"
        elif confidence < 0.7:
            return f"moderately likely {label}"
        elif confidence < 0.8:
            return f"probably {label}"
        elif confidence < 0.9:
            return f"fairly confident in {label}"
        elif confidence < 1.0:
            return f"pretty sure it's {label}"
        else:
            return f"Confirmed that it's {label}"

    @classmethod
    def get_message_content_from_labels(
        cls, labels: dict[str, float], min_confidence: float
    ) -> str:
        """Generate a message based on the input labels"""
        labeltext = "This is certainly bread! "
        for label, confidence in labels.items():
            if confidence >= min_confidence:
                labeltext = (
                    labeltext
                    + cls.map_confidence_to_sentiment(confidence=confidence, label=label)
                    + " "
                )
        logger.debug(labeltext)
        return labeltext

    @classmethod
    def get_message_from_roundness(cls, roundness: float | None = None) -> str:
        if roundness is None:
            return "I don't think this bread is round at all..."
        messagecontent = f"This bread seems {round(roundness * 100, 2):.2f}% round. Anything over an 80% is pretty close to a sphere!"
        logger.debug(messagecontent)
        return messagecontent

    @classmethod
    async def compute_bread_message_for_file(
        cls,
        input_file: Path,
        inference_client: InferenceClient,
        min_confidence: float,
    ) -> Tuple[Path, str, PredictResponse]:
        """Main "bread compute" function -> Does all the compute calls
        and returns the artifacts to be sent on the discord message"""

        payload = ImageData.from_img_path(input_file)
        res = await inference_client.predict(payload)
        # TODO: Min confidence is kinda broken right now
        if res.labels and "bread" in res.labels.keys():
            if res.labels["bread"] > SETTINGS.bread_detection_confidence:
                labels_comment = cls.get_message_content_from_labels(
                    labels=res.labels, min_confidence=min_confidence
                )
                if res.image:
                    out_path = SETTINGS.downloads_path / "predictions" / input_file.name
                    res.save_img(out_path)
                    roundness_comment = cls.get_message_from_roundness(res.roundness)
                    final_comment = labels_comment + roundness_comment
                else:
                    out_path = input_file
                    final_comment = (
                        labels_comment
                        + ". I couldn't find the shape dough. (Get it? Though - dough ehehehehe)"
                    )
                return out_path, final_comment, res
            else:
                # Bread was found but not confident enough
                final_comment = "This is only very mildly bread. Metaphysical bread even."
                return input_file, final_comment, res
        else:
            final_comment = "This isn't bread at all!"
            return input_file, final_comment, res

    @classmethod
    def is_bread_candidate(cls, message: discord.Message) -> bool:
        """Check Possible Bread Message: Message must be in one of the allowed channels
        made by a "Breadmancer" user and have attachments
        Args:
            message (DiscordMessage): DiscordMessage object

        Returns:
            Bool: Whether it passes all checks or not
        """

        logger.debug("Checking message for bread content...")
        # Check if the message is in an allowed channel
        if message.channel.id not in SETTINGS.discord_bread_channels:
            logger.debug("Message not in allowed channel")
            return False

        # Check if the author is from the specific group
        author = message.author
        author_role_ids = set([role.id for role in author.roles])
        allowed_role_ids = set(SETTINGS.discord_bread_role)

        if not bool(author_role_ids.intersection(allowed_role_ids)):
            logger.debug("Message not from correct author in group")
            return False

        # Check if there are any embedded pictures
        if len(message.attachments) == 0:
            logger.debug("Message without attachments")
            return False
        logger.debug("Bread message candidate detected")
        return True

    @classmethod
    def is_areyousure_message(cls, message: discord.Message, botuser: str) -> bool:
        """Check "Are you sure?" Message: User replies to message by bot and tells it
        to rerun the inference with lower confidence.
        Message can only be valid if it's a reply to a message done by the bot itself

        Args:
            message (DiscordMessage): DiscordMessage object
            botuser (str): bot user

        Returns:
            Bool: Whether it passes all checks or not
        """

        trigger_texts = ["are you sure", "no way"]
        logger.debug("Checking message for areyousure content...")
        # Check if the message is a reply
        if not (
            message.reference
            and message.reference.resolved
            and message.reference.resolved.author == botuser
        ):
            logger.info("Message is not a reply to the bot")
            return False
        # Check if message includes "are you sure or similar words"
        if not any(trigger in message.content.lower() for trigger in trigger_texts):
            logger.info("Message does not contain areyousure content")
            return False
        logger.debug("Message passes all checks")
        return True
